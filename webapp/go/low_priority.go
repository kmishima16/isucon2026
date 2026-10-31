package isureal

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

// 4 日目の山で asia2 の DB を取り合っていたのは、3 分の 2 が前からいる人の閲覧（画像・検索・Memories・フィードなど）だった。
// 翌日の新しい人の数は、新しい人が投稿を終えるまでの速さで決まると見ているので、重い閲覧だけ同時に動く数を絞り、新しい人の経路に DB を空ける。
// 待たせるのは上限の時間まで。b97df38 は 1 本の待ち行列で待たせ続け、走行後の確認の Memories が時間切れになって fail した
type lowPriorityLimiter struct {
	slots chan struct{}
	wait  time.Duration
}

// ISUREAL_LOW_PRIORITY_SLOTS が 0 か未設定なら絞らない（台ごとの drop-in で入り切りする）
func newLowPriorityLimiterFromEnv() *lowPriorityLimiter {
	n, err := strconv.Atoi(envOrDefault("ISUREAL_LOW_PRIORITY_SLOTS", "0"))
	if err != nil || n < 0 {
		panic("invalid ISUREAL_LOW_PRIORITY_SLOTS")
	}
	if n == 0 {
		return nil
	}
	ms, err := strconv.Atoi(envOrDefault("ISUREAL_LOW_PRIORITY_WAIT_MS", "1000"))
	if err != nil || ms < 0 {
		panic("invalid ISUREAL_LOW_PRIORITY_WAIT_MS")
	}
	return newLowPriorityLimiter(n, time.Duration(ms)*time.Millisecond)
}

func newLowPriorityLimiter(slots int, wait time.Duration) *lowPriorityLimiter {
	return &lowPriorityLimiter{slots: make(chan struct{}, slots), wait: wait}
}

// 新しい人の経路（登録・ログイン・友達申請・期間の取得・投稿）と、書き込み・友達フィード・通知・管理・初期化は絞らない
func isLowPriority(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	switch path {
	case "/api/posts/me/current":
		return false
	case "/api/memories", "/api/me/export", "/api/users",
		"/api/feeds/public", "/api/feeds/friends-of-friends", "/api/feeds/following":
		return true
	}
	for _, prefix := range []string{"/api/images/", "/api/memories/", "/api/users/", "/api/posts/", "/api/relationships/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func LowPriorityLimitMiddleware(l *lowPriorityLimiter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			// europe1・america1 から転送された閲覧は、欧米の翌日の人数が応答の速さで決まるので絞らない（asia1 の nginx が外からのこの値を消す）
			if l == nil || c.Request().Header.Get(proxiedRegionHeader) != "" || !isLowPriority(c.Request().Method, c.Request().URL.Path) {
				return next(c)
			}
			timer := time.NewTimer(l.wait)
			defer timer.Stop()
			select {
			case l.slots <- struct{}{}:
				defer func() { <-l.slots }()
			case <-timer.C:
				// 上限まで待ったら、枠を取らずに通す
			case <-c.Request().Context().Done():
				return c.Request().Context().Err()
			}
			return next(c)
		}
	}
}
