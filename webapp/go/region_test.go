package isureal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestRegionMiddlewareProxiedRegion(t *testing.T) {
	h := &HandlerContext{Config: &Config{Region: "asia"}}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	var proxied bool
	e.GET("/check", func(c *echo.Context) error {
		proxied = isProxiedRequest(c.Request().Context())
		return c.NoContent(http.StatusNoContent)
	}, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: "u", Region: c.Request().URL.Query().Get("session")})
			return next(c)
		}
	}, RegionMiddleware(h))

	cases := []struct {
		name          string
		sessionRegion string
		header        string
		wantCode      int
		wantProxied   bool
	}{
		{"同じリージョンの直接のリクエスト", "asia", "", http.StatusNoContent, false},
		{"ほかのリージョンの直接のリクエスト", "europe", "", http.StatusForbidden, false},
		{"europe1から転送されたeuropeの利用者", "europe", "europe", http.StatusNoContent, true},
		{"転送のリージョンとセッションが違う", "asia", "europe", http.StatusForbidden, false},
		{"リージョンとして正しくない値", "europe", "mars", http.StatusForbidden, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxied = false
			req := httptest.NewRequest(http.MethodGet, "/check?session="+tc.sessionRegion, nil)
			if tc.header != "" {
				req.Header.Set(proxiedRegionHeader, tc.header)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode || proxied != tc.wantProxied {
				t.Fatalf("code=%d proxied=%v; want %d %v (%s)", rec.Code, proxied, tc.wantCode, tc.wantProxied, rec.Body.String())
			}
		})
	}
}
