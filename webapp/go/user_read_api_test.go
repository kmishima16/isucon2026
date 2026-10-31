package isureal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// 初期実装の検索。速くした実装が同じ応答を返すことを確かめるために残す
func referenceNormalizeSearchText(value string) string {
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(value)))
}

func referenceSearchUsersHandler(h *HandlerContext) echo.HandlerFunc {
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
		var users []User
		if err := h.DB.SelectContext(ctx, &users, `SELECT `+strings.Join(userColumns, ", ")+` FROM users WHERE status='active' AND id<>$1 AND ($2='' OR (username,id)>($2,$3)) ORDER BY username, id`, viewer, input.BeforeUsername, input.BeforeID); err != nil {
			return err
		}
		blocked, err := getBlockedUserIDs(ctx, h.DB, viewer)
		if err != nil {
			return err
		}
		query := referenceNormalizeSearchText(input.Q)
		items := make([]relationshipUserResponse, 0)
		for _, user := range users {
			if !strings.Contains(referenceNormalizeSearchText(user.Username), query) && !strings.Contains(referenceNormalizeSearchText(user.DisplayName), query) {
				continue
			}
			if blocked[user.ID] {
				continue
			}
			if err := user.setImageKey(); err != nil {
				return err
			}
			if user.AccountType != "certified_public" {
				user.Bio = ""
			}
			items = append(items, newRelationshipUserResponse(user))
			if len(items) == 51 {
				break
			}
		}
		hasMore := len(items) > 50
		if hasMore {
			items = items[:50]
		}
		return c.JSON(http.StatusOK, searchUsersResponse{Items: items, HasMore: hasMore})
	}
}

// 正規化で一致が変わる表記（全角・大文字・合成済みと分解済み・ß・ケルビン記号）
var searchTestTexts = []string{
	"ＴＡＲＯ", "Taro", "해바라기", norm.NFD.String("해바라기"), "Cafe\u0301", "Straße", "\u212Aelvin", "山田 太郎",
}

func TestSearchTextMemo(t *testing.T) {
	m := newSearchTextMemo(3)
	inputs := append([]string{"", "Café", "ΣΊΣΥΦΟΣ", "İstanbul"}, searchTestTexts...)
	for range 3 {
		for _, value := range inputs {
			if got, want := m.normalize(value), referenceNormalizeSearchText(value); got != want {
				t.Fatalf("normalize(%q) = %q, want %q", value, got, want)
			}
			if len(m.entries) > 3 {
				t.Fatalf("memo grew to %d entries over the limit", len(m.entries))
			}
		}
	}
}

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/*.sql を流したもの）を渡したときだけ動く
func TestSearchUsersMatchesReference(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	db.MustExec(`TRUNCATE follows, friendships, friend_requests, block_relations, users CASCADE`)

	keyHash := func(id string) string {
		sum := sha256.Sum256([]byte(id))
		return hex.EncodeToString(sum[:])
	}
	insert := func(id, username, displayName, bio string, profileImageID *string, accountType, status string) {
		db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, bio, profile_image_id, region, account_type, status) VALUES ($1, $2, $3, $4, $5, $6, 'asia', $7, $8)`,
			id, username, keyHash(id), displayName, bio, profileImageID, accountType, status)
	}
	// id の順と行の物理的な順を username の順とずらし、id で読み直した行を元の順に並べ直せているかも確かめる
	idOf := func(i int) string { return fmt.Sprintf("u%03d", i*13%307) }
	// 閲覧者自身も検索語に当たるが、結果には出ない
	insert("viewer", "p150_taro_viewer", "Viewer Taro", "", nil, "regular", "active")
	for k := range 300 {
		i := k*7%300 + 1
		id := idOf(i)
		username := fmt.Sprintf("p%03d", i)
		if i%5 == 0 {
			username += "_taro"
		}
		var image *string
		if i%3 == 0 {
			imageID := "img-" + id
			image = &imageID
		}
		accountType := "regular"
		if i%7 == 0 {
			accountType = "certified_public"
		}
		status := "active"
		if i%13 == 0 {
			status = "deleted"
		}
		insert(id, username, fmt.Sprintf("%s %d", searchTestTexts[i%len(searchTestTexts)], i), fmt.Sprintf("bio %d", i), image, accountType, status)
	}
	// ブロックはどちら向きでも結果から外れる
	db.MustExec(`INSERT INTO block_relations (id, blocker_id, blocked_id) VALUES
	  ('b1', 'viewer', $1), ('b2', 'viewer', $2), ('b3', $3, 'viewer'), ('b4', $4, 'viewer'), ('b5', $5, $6)`,
		idOf(10), idOf(42), idOf(11), idOf(43), idOf(20), idOf(21))

	h := &HandlerContext{DB: db, Config: &Config{Region: "asia"}}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	asViewer := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: "viewer", Region: "asia"})
			return next(c)
		}
	}
	e.GET("/reference", referenceSearchUsersHandler(h), asViewer)
	e.GET("/api/users", searchUsersHandler(h), asViewer)
	get := func(path string, params url.Values) (int, string) {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"?"+params.Encode(), nil))
		return rec.Code, rec.Body.String()
	}
	// 画像 URL の期限は時刻で変わるので、初期実装を前後で 2 回呼んで同じだったときだけ比べる
	compare := func(params url.Values) string {
		for range 5 {
			refCode, refBody := get("/reference", params)
			code, body := get("/api/users", params)
			againCode, againBody := get("/reference", params)
			if refCode != againCode || refBody != againBody {
				continue
			}
			if code != refCode || body != refBody {
				t.Fatalf("params %v:\n got %d %s\nwant %d %s", params, code, body, refCode, refBody)
			}
			return body
		}
		t.Fatalf("params %v: reference response kept changing", params)
		return ""
	}

	pagesOf := map[string]int{}
	queries := []string{
		"taro", "TARO", "ｔａｒｏ", "Ｔａｒｏ", "해바라기", norm.NFD.String("해바라기"), "café", "CAFÉ", "cafe\u0301",
		"STRASSE", "ß", "kelvin", "太郎", "p", "1", "_", "bio", "viewer", "zzz",
	}
	for _, q := range queries {
		params := url.Values{"q": {q}}
		for page := 1; ; page++ {
			var res searchUsersResponse
			if err := json.Unmarshal([]byte(compare(params)), &res); err != nil {
				t.Fatalf("q %q page %d: %v", q, page, err)
			}
			pagesOf[q] = page
			if !res.HasMore {
				break
			}
			last := res.Items[len(res.Items)-1]
			params = url.Values{"q": {q}, "beforeUsername": {last.Username}, "beforeId": {last.ID}}
		}
	}
	// 検索語が正規化で当たり、ページをまたいでいることを確かめる（比べるだけでは、両方が空でも通る）
	for q, want := range map[string]int{"taro": 2, "해바라기": 2, "p": 6, "zzz": 1} {
		if pagesOf[q] != want {
			t.Fatalf("q %q: %d pages, want %d", q, pagesOf[q], want)
		}
	}

	// 途中のカーソル（実在しないユーザー名を含む）と、不正な入力
	for _, params := range []url.Values{
		{"q": {"p"}, "beforeUsername": {"p150"}, "beforeId": {"zzz"}},
		{"q": {"p"}, "beforeUsername": {"p149x"}, "beforeId": {"a"}},
		{"q": {"taro"}, "beforeUsername": {"p299"}, "beforeId": {"u299"}},
		{"q": {"taro"}, "beforeUsername": {"p100"}},
		{"q": {""}},
		{"q": {strings.Repeat("あ", 101)}},
	} {
		compare(params)
	}
}
