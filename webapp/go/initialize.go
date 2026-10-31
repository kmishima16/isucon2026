package isureal

import (
	"net/http"
	"os"
	"os/exec"

	"github.com/labstack/echo/v5"
)

type initializeResponse struct {
	Language string `json:"language"`
	Agent    string `json:"agent"`
	Harness  string `json:"harness"`
	Model    string `json:"model"`
}

func initializeHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		cmd := exec.Command("../sql/init.sh")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		// 検索は正規化した名前の列で絞る。初期化で戻した行と、列を足す前からの行は NULL なので埋める（user_read_api.go）
		if err := fillSearchTexts(c.Request().Context(), h.DB); err != nil {
			return err
		}
		// 初期化は利用者の行の updated_at を古い値に戻すので、検索の写しを持つ台に読み直させる（search_snapshot.go）
		if _, err := h.DB.ExecContext(c.Request().Context(), `UPDATE app_generation SET n = n + 1 WHERE id = 1`); err != nil {
			return err
		}
		// 走行中の投稿の画像は各台のディスクにある。DB から消えたので、全台の置き場所とメモリも空にする（image_store.go）
		images.purgeAll(c.Request().Context())

		// Set language to the implementation language.
		// Agents optimizing this app: set agent, harness, and model to the names used.
		// For multiple models, use comma-separated names; use brain when no AI model is used.
		return c.JSON(http.StatusOK, initializeResponse{Language: "go", Agent: "human", Harness: "manual", Model: "brain"})
	}
}
