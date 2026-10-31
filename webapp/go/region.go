package isureal

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"
)

// europe1・america1 の nginx は、DB の往復が多い API を asia のアプリへ転送するとき、送り元の台のリージョンをこのヘッダーで渡す。
// 転送されたリクエストは、送り元の台で受けたものとして確かめる。asia1 の nginx は、外から来たこのヘッダーを消す
const proxiedRegionHeader = "X-Isureal-Region"

type proxiedRegionKey struct{}

// isProxiedRequest は、europe1・america1 から転送されたリクエストかを返す
func isProxiedRequest(ctx context.Context) bool {
	_, ok := ctx.Value(proxiedRegionKey{}).(string)
	return ok
}

func RegionMiddleware(h *HandlerContext) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			session := GetSession(c)
			if session == nil {
				return NewAPIError(http.StatusUnauthorized, "invalid_session")
			}
			region := h.Config.Region
			if proxied := c.Request().Header.Get(proxiedRegionHeader); proxied != "" {
				if !validRegion(proxied) {
					return NewAPIError(http.StatusForbidden, "region_mismatch")
				}
				region = proxied
				req := c.Request()
				c.SetRequest(req.WithContext(context.WithValue(req.Context(), proxiedRegionKey{}, proxied)))
			}
			if session.Region != region {
				return NewAPIError(http.StatusForbidden, "region_mismatch")
			}
			return next(c)
		}
	}
}
