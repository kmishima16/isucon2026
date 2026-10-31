package isureal

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

type getNotificationsResponse struct {
	ID              string    `json:"id" db:"id"`
	PostingWindowID string    `json:"postingWindowId" db:"posting_window_id"`
	Region          string    `json:"region" db:"region"`
	CreatedAt       time.Time `json:"createdAt" db:"created_at"`
}

func getNotificationsHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		session := GetSession(c)
		userID := session.UserID
		// 接続の時刻は台の時計で取る。DB の時計を聞くと、europe1・america1 では SSE につなぐたびに往復 100ms かかっていた。
		// どの台の時計も Amazon Time Sync に合わせてあり、DB の台とのずれは 1ms 未満（chronyc tracking）
		connectedAt := time.Now()
		_, checkedUserGen, _ := h.Windows.snapshot()
		response := c.Response()
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusOK)
		controller := http.NewResponseController(response)
		if err := controller.Flush(); err != nil {
			return nil
		}
		for {
			window, userGen, changed := h.Windows.snapshot()
			due := window != nil && window.Region == session.Region && window.StartsAt.After(connectedAt)
			if due || userGen != checkedUserGen {
				// 開始のたびに DB で確かめる。ほかの台での退会やリージョンの移動は、この台の userGen では拾えない
				var recipient struct {
					Region    string    `db:"region"`
					CreatedAt time.Time `db:"created_at"`
				}
				err := h.DB.GetContext(ctx, &recipient, `SELECT region, created_at FROM users WHERE id = $1 AND status = 'active'`, userID)
				if err != nil {
					if !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
						c.Logger().Error("failed to get notification recipient", "error", err)
					}
					return nil
				}
				if recipient.Region != session.Region {
					return nil
				}
				checkedUserGen = userGen
				// 投稿期間の開始より後に登録した利用者には、その投稿期間の通知を作っていなかった
				if due && !recipient.CreatedAt.After(window.StartsAt) {
					data, err := json.Marshal(getNotificationsResponse{
						ID: notificationID(userID, window.ID), PostingWindowID: window.ID, Region: window.Region, CreatedAt: window.StartsAt,
					})
					if err != nil {
						c.Logger().Error("failed to encode notification", "error", err)
						return nil
					}
					if _, err := fmt.Fprintf(response, "data: %s\n\n", data); err != nil {
						return nil
					}
					_ = controller.Flush()
					return nil
				}
			}
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
			}
		}
	}
}
