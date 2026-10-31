package isureal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
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
	// 検索の候補は、検索 1 回分の大きな文字列（約 430KB）の部分文字列で渡ってくる。そのまま覚えると回ごとの文字列が手放されないので、複製して覚える
	key := strings.Clone(value)
	normalized = strings.Clone(normalizeSearchText(value))
	m.mu.Lock()
	if len(m.entries) >= m.limit {
		m.entries = make(map[string]string)
	}
	m.entries[key] = normalized
	m.mu.Unlock()
	return normalized
}

// 全員分の username と display_name を 1 つの文字列にまとめて毎回読んでいて、DB の時間の 2 位だった（1 回 57ms）。
// 正規化した名前の列（sql/011_users_search_text.sql）で DB が絞り、当たった人だけを全列で 1 回で読む。
// 正規化が NULL の行は DB では絞れないので必ず読み、アプリで今までどおり確かめる。
// 内側はインデックスだけで絞り（INCLUDE の列は Index Only Scan でしか条件に使えない）、外側で当たった行だけを主キーで表から読む（IN だと表を全件読んで突き合わせる）。
// カーソルの有無で SQL を分けるのは、($2=” OR ...) の形だと、使い回す実行計画がインデックスを使わず全件を並べ替えるため
const searchUsersQuery = `SELECT u.id, u.username, u.secret_key_hash, u.display_name, u.bio, u.profile_image_id, u.region, u.account_type, u.status
 FROM users u WHERE u.id = ANY(ARRAY(
  SELECT id FROM users WHERE status='active' AND id<>$1
   AND (search_username IS NULL OR search_display_name IS NULL OR strpos(search_username, $2) > 0 OR strpos(search_display_name, $2) > 0)
   AND NOT EXISTS (
    SELECT 1 FROM block_relations b
    WHERE (b.blocker_id = $1 AND b.blocked_id = users.id) OR (b.blocked_id = $1 AND b.blocker_id = users.id)
   )%s
  ORDER BY username, id LIMIT $3
 )) ORDER BY u.username, u.id`

var (
	searchUsersFirst = fmt.Sprintf(searchUsersQuery, "")
	searchUsersAfter = fmt.Sprintf(searchUsersQuery, ` AND (username,id)>($4,$5)`)
)

func selectSearchUsers(ctx context.Context, db sqlx.QueryerContext, viewer, query string, limit int, beforeUsername, beforeID string) ([]User, error) {
	var users []User
	var err error
	if beforeUsername == "" {
		err = sqlx.SelectContext(ctx, db, &users, searchUsersFirst, viewer, query, limit)
	} else {
		err = sqlx.SelectContext(ctx, db, &users, searchUsersAfter, viewer, query, limit, beforeUsername, beforeID)
	}
	return users, err
}

// fillSearchTexts は、正規化した名前が NULL の行を埋める（初期化の後）。読んだ時と名前が同じ行だけを書く
func fillSearchTexts(ctx context.Context, db *sqlx.DB) error {
	var rows []struct {
		ID          string `db:"id"`
		Username    string `db:"username"`
		DisplayName string `db:"display_name"`
	}
	if err := db.SelectContext(ctx, &rows, `SELECT id, username, display_name FROM users WHERE search_username IS NULL OR search_display_name IS NULL`); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, len(rows))
	usernames := make([]string, len(rows))
	displayNames := make([]string, len(rows))
	searchUsernames := make([]string, len(rows))
	searchDisplayNames := make([]string, len(rows))
	for i, row := range rows {
		ids[i], usernames[i], displayNames[i] = row.ID, row.Username, row.DisplayName
		searchUsernames[i] = normalizeSearchText(row.Username)
		searchDisplayNames[i] = normalizeSearchText(row.DisplayName)
	}
	_, err := db.ExecContext(ctx, `
  UPDATE users u SET search_username = v.su, search_display_name = v.sd
  FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[]) AS v(id, un, dn, su, sd)
  WHERE u.id = v.id AND u.username = v.un AND u.display_name = v.dn
 `, ids, usernames, displayNames, searchUsernames, searchDisplayNames)
	return err
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
		query := normalizeSearchText(input.Q)
		// ブロックはどちら向きでも、DB が絞るときに外す（europe1・america1 では別に読むと 1 往復 100ms 増える）。
		// DB が絞った行のうち外れるのは、正規化が NULL で当たらない人だけ。足りなければ続きを読む
		const limit = 51
		matched := make([]User, 0, 51)
		beforeUsername, beforeID := input.BeforeUsername, input.BeforeID
		if h.SearchUsers != nil {
			// 利用者の写しで照合する（search_snapshot.go）
			var err error
			if matched, err = h.SearchUsers.search(ctx, h.ReadDB, viewer, query, beforeUsername, beforeID, limit); err != nil {
				return err
			}
		}
	scan:
		for h.SearchUsers == nil {
			users, err := selectSearchUsers(ctx, h.ReadDB, viewer, query, limit, beforeUsername, beforeID)
			if err != nil {
				return err
			}
			for _, user := range users {
				if !strings.Contains(searchTexts.normalize(user.Username), query) && !strings.Contains(searchTexts.normalize(user.DisplayName), query) {
					continue
				}
				matched = append(matched, user)
				if len(matched) == 51 {
					break scan
				}
			}
			if len(users) < limit {
				break
			}
			beforeUsername, beforeID = users[len(users)-1].Username, users[len(users)-1].ID
		}
		jobs := make([]signJob, 0, len(matched))
		for i := range matched {
			if err := matched[i].setImageKey(); err != nil {
				return err
			}
			jobs = appendProfileImageJob(jobs, matched[i])
		}
		// 結果のプロフィール画像の署名を、europe1・america1 にまとめて頼む（image_signer.go）
		prefetchImageSignatures(ctx, jobs)
		items := make([]relationshipUserResponse, 0, len(matched))
		for _, user := range matched {
			if user.AccountType != "certified_public" {
				user.Bio = ""
			}
			items = append(items, newRelationshipUserResponse(user))
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
		user, err := getUser(ctx, h.ReadDB, input.UserID)
		if errors.Is(err, sql.ErrNoRows) {
			return NewAPIError(http.StatusNotFound, "relationship_not_found")
		}
		if err != nil {
			return err
		}
		blocked, err := isBlocked(ctx, h.ReadDB, viewer, user.ID)
		if err != nil {
			return err
		}
		if blocked {
			return NewAPIError(http.StatusNotFound, "relationship_not_found")
		}
		friends, err := getFriendIDs(ctx, h.ReadDB, viewer)
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
