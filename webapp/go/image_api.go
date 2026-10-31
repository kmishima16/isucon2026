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
		state, cached := h.Images.state(input.ImageID)
		data, haveData := h.Images.getData(input.ImageID)
		if !cached {
			// 削除状態は imageStateTTL ごとに確認する。即時反映も5秒以内の許容範囲に含まれる。
			var row struct {
				SecretKeyHash string `db:"secret_key_hash"`
				MediaType     string `db:"media_type"`
				OwnerActive   bool   `db:"owner_active"`
				Referenced    bool   `db:"referenced"`
				Data          []byte `db:"data"`
			}
			err = h.DB.GetContext(ctx, &row, `
   SELECT u.secret_key_hash, i.media_type, u.status = 'active' AS owner_active,
     COALESCE(u.profile_image_id = i.id, false)
     OR EXISTS (
       SELECT 1 FROM posts p
       WHERE p.image_id = i.id AND p.author_id = u.id AND p.deleted_at IS NULL
     )
     OR EXISTS (
       SELECT 1 FROM realmojis r
       JOIN posts p ON p.id = r.post_id AND p.deleted_at IS NULL
       JOIN users author ON author.id = p.author_id AND author.status = 'active'
       WHERE r.image_id = i.id AND r.reactor_id = u.id AND r.replaced_at IS NULL AND r.deleted_at IS NULL
     ) AS referenced,
     CASE WHEN $2 THEN NULL ELSE i.data END AS data
   FROM image_objects i JOIN users u ON u.id = i.owner_user_id
   WHERE i.id = $1
  `, input.ImageID, haveData)
			if errors.Is(err, sql.ErrNoRows) {
				return NewAPIError(http.StatusForbidden, "image_forbidden")
			}
			if err != nil {
				return err
			}
			key, err := imageSigningKey(row.SecretKeyHash)
			if err != nil {
				return err
			}
			state = imageState{key: key, mediaType: row.MediaType, ownerActive: row.OwnerActive, referenced: row.Referenced, checkedAt: time.Now()}
			h.Images.setState(input.ImageID, state)
			if !haveData {
				data, haveData = row.Data, true
				h.Images.setData(input.ImageID, data)
			}
		}
		if !state.ownerActive {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if !verifyImageSignature(state.key, input.ImageID, exp, sig) {
			c.Set("abuse", "invalid_signature")
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if !state.referenced {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if !haveData {
			if err := h.DB.GetContext(ctx, &data, `SELECT data FROM image_objects WHERE id = $1`, input.ImageID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return NewAPIError(http.StatusForbidden, "image_forbidden")
				}
				return err
			}
			h.Images.setData(input.ImageID, data)
		}
		if exp <= time.Now().Unix() {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		return c.Blob(http.StatusOK, state.mediaType, data)
	}
}
