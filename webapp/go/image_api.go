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
		// 中身はこの台が持っていれば読まない（image_data_cache.go）。初期データの投稿の画像は、書き出したディスクから読む（initial_image_store.go）
		cached, haveData := h.ImageData.get(input.ImageID)
		if !haveData {
			if cached, haveData = initialImages.read(input.ImageID); haveData {
				h.ImageData.set(input.ImageID, cached)
			}
		}
		// 削除状態を毎回確認する。即時反映も5秒以内の許容範囲に含まれる
		object, err := getServableImage(ctx, h.ReadDB, input.ImageID, haveData)
		if errors.Is(err, sql.ErrNoRows) {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if err != nil {
			return err
		}
		key, err := imageSigningKey(object.SecretKeyHash)
		if err != nil {
			return err
		}
		// 持ち主が退会した画像は上で 403 にしてあり、不正な署名として記録しない（今までと同じ順）
		if !verifyImageSignature(key, input.ImageID, exp, sig) {
			c.Set("abuse", "invalid_signature")
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if !object.Live {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		switch {
		case haveData:
			object.Data = cached
		case object.Host != nil:
			if object.Data, err = images.load(ctx, *object.Host, input.ImageID); err != nil {
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
