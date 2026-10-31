package isureal

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

type countingBody struct {
	r    io.Reader
	read int
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.read += n
	return n, err
}

func (b *countingBody) Close() error { return nil }

func TestRejectedRequestBodyIsDrained(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	e.Use(DrainRequestBodyMiddleware())
	e.PUT("/api/me/profile-image", func(c *echo.Context) error {
		return NewAPIError(http.StatusForbidden, "ip_banned")
	})

	size := 1 << 20
	body := &countingBody{r: bytes.NewReader(make([]byte, size))}
	req := httptest.NewRequest(http.MethodPut, "/api/me/profile-image", body)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if body.read != size {
		t.Fatalf("read %d bytes of the body, want %d", body.read, size)
	}
}
