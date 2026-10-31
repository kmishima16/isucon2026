package isureal

import (
	"github.com/labstack/echo/v5"
)

// Memories とエクスポートは、1 回で数十〜数百の画像 URL に署名し、asia1 の CPU の半分ほどを使っていた。
// 同時に何本も動くと 2 コアを埋め、投稿期間の途中で登録した人の登録・ログイン・友達申請・投稿が 10 倍遅くなり、期間内に投稿できなかった（logs/notes/bench-results.md の 40100 点の節）。
// この 2 つの同時実行を台ごとに絞り、残りの CPU を新しく来た人の経路に空ける。待っている間に接続が切れたら、待つのをやめる
const heavyConcurrency = 1

func HeavyLimitMiddleware(limit int) echo.MiddlewareFunc {
	slots := make(chan struct{}, limit)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := c.Request().Context()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			defer func() { <-slots }()
			return next(c)
		}
	}
}
