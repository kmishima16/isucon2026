package isureal

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"
)

type getImageRequest struct {
	ImageID string `param:"imageId" json:"-"`
	Exp     string `query:"exp" json:"-"`
	Sig     string `query:"sig" json:"-"`
}

func getImageHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-store")
		var input getImageRequest
		if err := c.Bind(&input); err != nil {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		query, err := url.ParseQuery(c.Request().URL.RawQuery)
		if err != nil || len(query["exp"]) != 1 || len(query["sig"]) != 1 {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		exp, err := strconv.ParseInt(input.Exp, 10, 64)
		if err != nil || strconv.FormatInt(exp, 10) != input.Exp || exp <= time.Now().Unix() {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		sig, err := base64.RawURLEncoding.Strict().DecodeString(input.Sig)
		if err != nil || len(sig) != sha256.Size || base64.RawURLEncoding.EncodeToString(sig) != input.Sig {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}

		ctx := c.Request().Context()
		owner, cachedOwner := h.ImageOwners.get(input.ImageID)
		if cachedOwner {
			key, err := imageSigningKey(owner.SecretKeyHash)
			if err != nil {
				return err
			}
			// 署名が合わないときは、持ち主が退会していないかを DB で確かめてから扱いを決める（退会した人の画像は不正な署名として記録しない）
			cachedOwner = verifyImageSignature(key, input.ImageID, exp, sig)
		}
		if !cachedOwner {
			owner, err = getImageOwnerKey(ctx, h.DB, input.ImageID)
			if errors.Is(err, sql.ErrNoRows) {
				return NewAPIError(http.StatusForbidden, "image_forbidden")
			}
			if err != nil {
				return err
			}
			key, err := imageSigningKey(owner.SecretKeyHash)
			if err != nil {
				return err
			}
			if !verifyImageSignature(key, input.ImageID, exp, sig) {
				c.Set("abuse", "invalid_signature")
				return NewAPIError(http.StatusForbidden, "image_forbidden")
			}
			h.ImageOwners.set(input.ImageID, owner)
		}

		// 削除状態を毎回確認する。即時反映も5秒以内の許容範囲に含まれる。
		// 中身はこの台が持っていれば読まない（image_data_cache.go）
		cached, haveData := h.ImageData.get(input.ImageID)
		external := owner.Host != nil
		var object struct {
			Data      []byte `db:"data"`
			MediaType string `db:"media_type"`
		}
		err = h.DB.GetContext(ctx, &object, `
   SELECT CASE WHEN $2 THEN NULL ELSE i.data END AS data, i.media_type
   FROM image_objects i JOIN users u ON u.id = i.owner_user_id AND u.status = 'active'
   WHERE i.id = $1 AND i.created_at = $3 AND (
     u.profile_image_id = i.id
     OR EXISTS (
       SELECT 1 FROM posts p
       WHERE p.image_id = i.id AND p.author_id = u.id AND p.deleted_at IS NULL
     )
     OR EXISTS (
       SELECT 1 FROM realmojis r
       JOIN posts p ON p.id = r.post_id AND p.deleted_at IS NULL
       JOIN users author ON author.id = p.author_id AND author.status = 'active'
       WHERE r.image_id = i.id AND r.reactor_id = u.id AND r.replaced_at IS NULL AND r.deleted_at IS NULL
     )
   )
  `, input.ImageID, haveData || external, owner.CreatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if err != nil {
			return err
		}
		switch {
		case haveData:
			object.Data = cached
		case external:
			if object.Data, err = images.load(ctx, *owner.Host, input.ImageID); err != nil {
				return err
			}
		default:
			h.ImageData.set(input.ImageID, object.Data)
		}
		if exp <= time.Now().Unix() {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		return c.Blob(http.StatusOK, object.MediaType, object.Data)
	}
}
