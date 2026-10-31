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

// 画像の本文（数百 KB）を io.ReadAll で読むと、足りなくなるたびに取り直してゼロで埋め、
// asia1 のプロフィール画像の CPU の 8 割がこれだった（17:26 の pprof）。大きさが分かるときは一度に確保する
func readAllSized(r io.Reader, size int64) ([]byte, error) {
	if size <= 0 || size > maxDrainedRequestBody {
		return io.ReadAll(r)
	}
	data := make([]byte, 0, size+1)
	for {
		n, err := r.Read(data[len(data):cap(data)])
		data = data[:len(data)+n]
		if err == io.EOF {
			return data, nil
		}
		if err != nil {
			return data, err
		}
		if len(data) == cap(data) {
			// 申告より長い本文は、残りを今までどおり読む
			rest, err := io.ReadAll(r)
			return append(data, rest...), err
		}
	}
}
