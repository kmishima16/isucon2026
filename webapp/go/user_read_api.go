package isureal

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type searchUsersRequest struct {
	Q              string `query:"q"`
	BeforeUsername string `query:"beforeUsername"`
	BeforeID       string `query:"beforeId"`
}

type searchUsersResponse struct {
	Items   []relationshipUserResponse `json:"items"`
	HasMore bool                       `json:"hasMore"`
}

func normalizeSearchText(value string) string {
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(value)))
}

// 検索のたびに全員の username と display_name を正規化していて、asia の CPU の 1 割を使っていた。
// 同じ文字列は必ず同じ結果になるので、台ごとのメモリに覚えておく（台の間で食い違わず、初期化で消す必要もない）
type searchTextMemo struct {
	mu      sync.RWMutex
	limit   int
	entries map[string]string
}

func newSearchTextMemo(limit int) *searchTextMemo {
	return &searchTextMemo{limit: limit, entries: make(map[string]string)}
}

// 登録や名前の変更で古い文字列が残り続けるので、上限を超えたら覚え直す
var searchTexts = newSearchTextMemo(200000)

func (m *searchTextMemo) normalize(value string) string {
	m.mu.RLock()
	normalized, ok := m.entries[value]
	m.mu.RUnlock()
	if ok {
		return normalized
	}
	normalized = normalizeSearchText(value)
	m.mu.Lock()
	if len(m.entries) >= m.limit {
		m.entries = make(map[string]string)
	}
	m.entries[value] = normalized
	m.mu.Unlock()
	return normalized
}

type searchCandidate struct {
	ID          string `db:"id"`
	Username    string `db:"username"`
	DisplayName string `db:"display_name"`
}

// 有効なユーザー全員を全列で読んで並べ替えていて、1 回 82ms かかっていた。照合に要る 3 列だけを
// sql/003_users_search.sql のインデックスから並んだまま読む。カーソルの有無で SQL を分けるのは、
// ($2=” OR ...) の形だと、使い回す実行計画がインデックスを使わず全件を並べ替えるため。
// 約 5000 行を 1 行ずつ受け取って詰めるだけで asia1 の CPU の 2 割を使っていたので、全員分を「バイト数:中身」の並びの
// 1 つの文字列にまとめて受け取る。表示名にはどんな文字も入るので、区切り文字ではなく長さで切る
const searchCandidatesPacked = `SELECT coalesce(string_agg(octet_length(id) || ':' || id || octet_length(username) || ':' || username || octet_length(display_name) || ':' || display_name, '' ORDER BY username, id), '') FROM users WHERE status='active' AND id<>$1`

func selectSearchCandidates(ctx context.Context, db sqlx.QueryerContext, viewer, beforeUsername, beforeID string) ([]searchCandidate, error) {
	var packed string
	var err error
	if beforeUsername == "" {
		err = sqlx.GetContext(ctx, db, &packed, searchCandidatesPacked, viewer)
	} else {
		err = sqlx.GetContext(ctx, db, &packed, searchCandidatesPacked+` AND (username,id)>($2,$3)`, viewer, beforeUsername, beforeID)
	}
	if err != nil {
		return nil, err
	}
	return parseSearchCandidates(packed)
}

var errMalformedSearchCandidates = errors.New("malformed search candidates")

// 切り出した値は packed を共有するので、1 人ごとにメモリを確保しない
func parseSearchCandidates(packed string) ([]searchCandidate, error) {
	candidates := make([]searchCandidate, 0, len(packed)/48)
	rest := packed
	next := func() (string, bool) {
		colon := strings.IndexByte(rest, ':')
		if colon <= 0 {
			return "", false
		}
		n, err := strconv.Atoi(rest[:colon])
		if err != nil || n < 0 || colon+1+n > len(rest) {
			return "", false
		}
		value := rest[colon+1 : colon+1+n]
		rest = rest[colon+1+n:]
		return value, true
	}
	for rest != "" {
		id, ok1 := next()
		username, ok2 := next()
		displayName, ok3 := next()
		if !ok1 || !ok2 || !ok3 {
			return nil, errMalformedSearchCandidates
		}
		candidates = append(candidates, searchCandidate{ID: id, Username: username, DisplayName: displayName})
	}
	return candidates, nil
}

func searchUsersHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var input searchUsersRequest
		if err := c.Bind(&input); err != nil {
			return err
		}
		if !validUserText(input.Q, 1, 100) {
			return NewAPIError(http.StatusBadRequest, "invalid_relationship")
		}
		if (input.BeforeUsername == "") != (input.BeforeID == "") {
			return NewAPIError(http.StatusBadRequest, "invalid_pagination")
		}
		ctx := c.Request().Context()
		viewer := GetSession(c).UserID
		candidates, err := selectSearchCandidates(ctx, h.DB, viewer, input.BeforeUsername, input.BeforeID)
		if err != nil {
			return err
		}
		blocked, err := getBlockedUserIDs(ctx, h.DB, viewer)
		if err != nil {
			return err
		}
		query := normalizeSearchText(input.Q)
		ids := make([]string, 0, 51)
		for _, candidate := range candidates {
			if !strings.Contains(searchTexts.normalize(candidate.Username), query) && !strings.Contains(searchTexts.normalize(candidate.DisplayName), query) {
				continue
			}
			if blocked[candidate.ID] {
				continue
			}
			ids = append(ids, candidate.ID)
			if len(ids) == 51 {
				break
			}
		}
		items := make([]relationshipUserResponse, 0, len(ids))
		if len(ids) > 0 {
			var users []User
			if err := h.DB.SelectContext(ctx, &users, `SELECT `+strings.Join(userColumns, ", ")+` FROM users WHERE id = ANY($1) AND status='active'`, ids); err != nil {
				return err
			}
			byID := make(map[string]User, len(users))
			jobs := make([]signJob, 0, len(users))
			for i := range users {
				if err := users[i].setImageKey(); err != nil {
					return err
				}
				byID[users[i].ID] = users[i]
				jobs = appendProfileImageJob(jobs, users[i])
			}
			// 結果のプロフィール画像の署名を、europe1・america1 にまとめて頼む（image_signer.go）
			prefetchImageSignatures(ctx, jobs)
			for _, id := range ids {
				// 2 回の読み取りの間に退会したユーザーは出さない
				user, ok := byID[id]
				if !ok {
					continue
				}
				if user.AccountType != "certified_public" {
					user.Bio = ""
				}
				items = append(items, newRelationshipUserResponse(user))
			}
		}
		hasMore := len(items) > 50
		if hasMore {
			items = items[:50]
		}
		return c.JSON(http.StatusOK, searchUsersResponse{Items: items, HasMore: hasMore})
	}
}

type getUserRequest struct {
	UserID string `param:"userId"`
}

type getUserResponse relationshipUserResponse

func getUserHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var input getUserRequest
		if err := c.Bind(&input); err != nil {
			return err
		}
		ctx := c.Request().Context()
		viewer := GetSession(c).UserID
		user, err := getUser(ctx, h.DB, input.UserID)
		if errors.Is(err, sql.ErrNoRows) {
			return NewAPIError(http.StatusNotFound, "relationship_not_found")
		}
		if err != nil {
			return err
		}
		blocked, err := isBlocked(ctx, h.DB, viewer, user.ID)
		if err != nil {
			return err
		}
		if blocked {
			return NewAPIError(http.StatusNotFound, "relationship_not_found")
		}
		friends, err := getFriendIDs(ctx, h.DB, viewer)
		if err != nil {
			return err
		}
		if user.AccountType != "certified_public" && !slices.Contains(friends, user.ID) {
			user.Bio = ""
		}
		return c.JSON(http.StatusOK, getUserResponse(newRelationshipUserResponse(user)))
	}
}

func newRelationshipUserResponse(user User) relationshipUserResponse {
	return relationshipUserResponse{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, ProfileImageURL: newGetMeResponse(user).ProfileImageURL, Bio: user.Bio, AccountType: user.AccountType}
}
