package isureal

import (
	"io"

	"github.com/labstack/echo/v5"
)

// nginx の client_max_body_size と同じ
const maxDrainedRequestBody = 32 << 20

// BAN や認証で本文を読まずに返すと、Go のサーバーは読み残しが大きい接続を閉じる。
// nginx は本文を送っている途中で切られて 502 を返し、403 などの応答が届かなかった（#6）。
// 応答を書く前に残りを読み切って、接続を閉じずに済ませる
func DrainRequestBodyMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			if body := c.Request().Body; body != nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrainedRequestBody))
			}
			return err
		}
	}
}
