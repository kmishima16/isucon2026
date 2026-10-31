package isureal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// 初期実装のフィードの候補。開いている投稿期間の投稿を全部読んでから Go で絞る。SQL で絞った実装が同じ候補を返すことを確かめるために残す
func referenceFeedCandidates(ctx context.Context, db *sqlx.DB, viewer viewerContext, feed string, beforePostedAt *time.Time, beforeID string) ([]PostWithAuthor, error) {
	following := map[string]bool{}
	if feed == "following" {
		var followeeIDs []string
		if err := db.SelectContext(ctx, &followeeIDs, `SELECT followee_id FROM follows WHERE follower_id = $1`, viewer.ID); err != nil {
			return nil, err
		}
		for _, id := range followeeIDs {
			following[id] = true
		}
	}
	posts, err := selectPostsWithAuthor(ctx, db, `
   AND p.author_id <> $1 AND w.ended_at IS NULL
   AND ($2::timestamptz IS NULL OR (p.posted_at, p.id) < ($2, $3))
   ORDER BY p.posted_at DESC, p.id DESC
  `, viewer.ID, beforePostedAt, beforeID)
	if err != nil {
		return nil, err
	}
	candidates := make([]PostWithAuthor, 0)
	for _, post := range posts {
		if viewer.canView(post.Post) && matchesFeed(viewer, following, post.Post, feed) {
			candidates = append(candidates, post)
		}
	}
	return candidates, nil
}

// 初期実装の getFeedHandler の、利用者を読んだ後の部分
func referenceFeed(ctx context.Context, db *sqlx.DB, viewerID string, input getFeedRequest) (getFeedResponse, error) {
	viewer, err := loadViewerContext(ctx, db, viewerID)
	if errors.Is(err, sql.ErrNoRows) {
		return getFeedResponse{Items: []feedItemResponse{}}, nil
	}
	if err != nil {
		return getFeedResponse{}, err
	}
	ranked := input.Feed == "friends-of-friends"
	beforePostedAt, beforeID := input.BeforePostedAt, input.BeforeID
	if ranked {
		beforePostedAt, beforeID = nil, ""
	}
	candidates, err := referenceFeedCandidates(ctx, db, viewer, input.Feed, beforePostedAt, beforeID)
	if err != nil {
		return getFeedResponse{}, err
	}
	var scores map[string]int64
	if ranked {
		if scores, err = recommendationScores(ctx, db, viewer, candidates); err != nil {
			return getFeedResponse{}, err
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			return scores[candidates[i].ID] > scores[candidates[j].ID]
		})
	}
	items := make([]feedItemResponse, 0)
	for _, post := range candidates {
		score := scores[post.ID]
		if ranked && input.BeforeScore != nil && !rankedBefore(score, post.Post, *input.BeforeScore, *input.BeforePostedAt, input.BeforeID) {
			continue
		}
		item := feedItemResponse{createPostResponse: newPostResponse(post.Post, post.Author)}
		if ranked {
			item.Score = &score
		}
		items = append(items, item)
		if len(items) == 51 {
			break
		}
	}
	hasMore := len(items) > 50
	if hasMore {
		items = items[:50]
	}
	return getFeedResponse{Items: items, HasMore: hasMore}, nil
}

var feedTypes = []string{"friends", "friends-of-friends", "public", "following"}

// ISUREAL_TEST_DSN に空の DB（001-schema.sql を流したもの）を渡したときだけ動く
func TestFeedMatchesReference(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	viewers := seedFeedFixture(t, db)
	ctx := context.Background()

	t.Run("candidates", func(t *testing.T) {
		nonEmpty := map[string]bool{}
		for _, viewerID := range viewers {
			viewer, err := loadViewerContext(ctx, db, viewerID)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, feed := range feedTypes {
				all, err := referenceFeedCandidates(ctx, db, viewer, feed, nil, "")
				if err != nil {
					t.Fatal(err)
				}
				if len(all) > 0 {
					nonEmpty[feed] = true
				}
				// 先頭から、途中の投稿の位置から、最後の投稿の位置から、どの投稿とも時刻が違う位置から
				type cursor struct {
					postedAt *time.Time
					id       string
				}
				cursors := []cursor{{nil, ""}}
				for _, i := range []int{len(all) / 2, len(all) - 1} {
					if i >= 0 && i < len(all) {
						cursors = append(cursors, cursor{&all[i].PostedAt, all[i].ID})
						between := all[i].PostedAt.Add(500 * time.Millisecond)
						cursors = append(cursors, cursor{&between, ""})
					}
				}
				for _, c := range cursors {
					want, err := referenceFeedCandidates(ctx, db, viewer, feed, c.postedAt, c.id)
					if err != nil {
						t.Fatal(err)
					}
					got, err := selectFeedCandidates(ctx, db, viewer, feed, c.postedAt, c.id)
					if err != nil {
						t.Fatalf("%s %s: %v", viewerID, feed, err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("%s %s cursor %v %q:\n got %v\nwant %v", viewerID, feed, c.postedAt, c.id, postIDs(got), postIDs(want))
					}
				}
			}
		}
		for _, feed := range feedTypes {
			if !nonEmpty[feed] {
				t.Fatalf("fixture has no candidates for %s", feed)
			}
		}
	})

	t.Run("handler", func(t *testing.T) {
		h := &HandlerContext{DB: db, Config: &Config{Region: "asia"}}
		e := echo.New()
		e.HTTPErrorHandler = APIErrorHandler
		e.GET("/api/feeds/:feed", getFeedHandler(h), func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				c.Set("session", &Session{UserID: c.Request().Header.Get("X-Test-User"), Region: "asia"})
				return next(c)
			}
		})
		paged := map[string]bool{}
		for _, viewerID := range viewers {
			for _, feed := range feedTypes {
				input := getFeedRequest{Feed: feed}
				query := url.Values{}
				for page := 0; ; page++ {
					req := httptest.NewRequest(http.MethodGet, "/api/feeds/"+feed+"?"+query.Encode(), nil)
					req.Header.Set("X-Test-User", viewerID)
					rec := httptest.NewRecorder()
					e.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("%s %s page %d: %d %s", viewerID, feed, page, rec.Code, rec.Body.String())
					}
					var got getFeedResponse
					if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					want, err := referenceFeed(ctx, db, viewerID, input)
					if err != nil {
						t.Fatal(err)
					}
					wantJSON, _ := json.Marshal(want)
					want = getFeedResponse{}
					if err := json.Unmarshal(wantJSON, &want); err != nil {
						t.Fatal(err)
					}
					// 画像 URL の期限は呼んだ時刻で変わるので比べない
					for _, items := range [][]feedItemResponse{got.Items, want.Items} {
						for i := range items {
							items[i].ImageURL = ""
							items[i].Author.ProfileImageURL = nil
						}
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("%s %s page %d:\n got %v hasMore=%v\nwant %v hasMore=%v", viewerID, feed, page, itemIDs(got.Items), got.HasMore, itemIDs(want.Items), want.HasMore)
					}
					if !got.HasMore {
						break
					}
					paged[feed] = true
					last := got.Items[len(got.Items)-1]
					input.BeforePostedAt, input.BeforeID = &last.PostedAt, last.ID
					query = url.Values{"beforePostedAt": {last.PostedAt.Format(time.RFC3339Nano)}, "beforeId": {last.ID}}
					if feed == "friends-of-friends" {
						input.BeforeScore = last.Score
						query.Set("beforeScore", strconv.FormatInt(*last.Score, 10))
					}
				}
			}
		}
		for _, feed := range feedTypes {
			if !paged[feed] {
				t.Fatalf("fixture never paginates %s", feed)
			}
		}
	})
}

func postIDs(posts []PostWithAuthor) []string {
	ids := make([]string, len(posts))
	for i, post := range posts {
		ids[i] = post.ID
	}
	return ids
}

func itemIDs(items []feedItemResponse) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
		if item.Score != nil {
			ids[i] += ":" + strconv.FormatInt(*item.Score, 10)
		}
	}
	return ids
}

// 友達・友達の友達・フォロー・ブロック・退会・公開範囲・削除済み・終わった投稿期間・複数リージョンの開いている投稿期間を混ぜ、
// どのフィードも 50 件を超えてページが分かれるだけの投稿を作る。返すのはフィードを見る利用者
func seedFeedFixture(t *testing.T, db *sqlx.DB) []string {
	t.Helper()
	db.MustExec(`TRUNCATE users, posting_windows CASCADE`)
	rng := rand.New(rand.NewPCG(9, 26))
	tx := db.MustBegin()
	defer tx.Rollback()
	const key = "8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4"
	base := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

	// asia と europe は 10/31、america は 10/30 の投稿期間が開いている
	windows := []struct {
		id, region, date string
		ended            bool
	}{
		{"w-asia-open", "asia", "2026-10-31", false},
		{"w-europe-open", "europe", "2026-10-31", false},
		{"w-america-open", "america", "2026-10-30", false},
		{"w-asia-ended", "asia", "2026-10-30", true},
		{"w-europe-ended", "europe", "2026-10-30", true},
		{"w-america-ended", "america", "2026-10-29", true},
	}
	openWindow := map[string]string{}
	endedWindow := map[string]string{}
	for _, w := range windows {
		var endedAt *time.Time
		if w.ended {
			ended := base.Add(-time.Hour)
			endedAt = &ended
			endedWindow[w.region] = w.id
		} else {
			openWindow[w.region] = w.id
		}
		tx.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline, ended_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			w.id, w.region, w.date, base, base.Add(20*time.Second), endedAt)
	}

	regions := []string{"asia", "asia", "asia", "europe", "america"}
	type user struct{ id, region, accountType, status string }
	special := []user{
		{"v", "asia", "regular", "active"},       // 今日投稿した。友達も友達の友達もフォローも多い
		{"lazy", "asia", "regular", "active"},    // 今日は投稿していない（終わった投稿期間にだけ投稿した）
		{"undone", "asia", "regular", "active"},  // 今日の投稿を消した
		{"eu", "europe", "regular", "active"},    // europe で今日投稿した
		{"am", "america", "regular", "active"},   // america は日付が 1 日遅い
		{"loner", "europe", "regular", "active"}, // 友達もフォローもいない
		{"gone", "asia", "regular", "deleted"},   // 退会済み
		{"star", "asia", "certified_public", "active"},
	}
	users := append([]user{}, special...)
	var others []string
	for i := range 300 {
		u := user{fmt.Sprintf("u%03d", i), regions[rng.IntN(len(regions))], "regular", "active"}
		if rng.IntN(100) < 15 {
			u.accountType = "certified_public"
		}
		if rng.IntN(100) < 5 {
			u.status = "deleted"
		}
		users = append(users, u)
		others = append(others, u.id)
	}
	for _, u := range users {
		var profileImageID *string
		if rng.IntN(2) == 0 {
			id := "profile-" + u.id
			profileImageID = &id
		}
		tx.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, bio, profile_image_id, region, account_type, status) VALUES ($1, $1, $2, $3, $4, $5, $6, $7, $8)`,
			u.id, key, "D "+u.id, "bio "+u.id, profileImageID, u.region, u.accountType, u.status)
	}

	friendships := map[[2]string]bool{}
	befriend := func(a, b string) {
		if a == b {
			return
		}
		pair := [2]string{min(a, b), max(a, b)}
		if friendships[pair] {
			return
		}
		friendships[pair] = true
		tx.MustExec(`INSERT INTO friendships (id, user_low_id, user_high_id) VALUES ($1, $2, $3)`, pair[0]+"-"+pair[1], pair[0], pair[1])
	}
	pick := func(n int) []string {
		perm := rng.Perm(len(others))
		ids := make([]string, n)
		for i := range n {
			ids[i] = others[perm[i]]
		}
		return ids
	}
	vFriends := pick(100)
	for _, id := range vFriends {
		befriend("v", id)
	}
	for _, id := range pick(40) {
		befriend("lazy", id)
	}
	for _, id := range pick(30) {
		befriend("eu", id)
	}
	for _, id := range pick(20) {
		befriend("am", id)
	}
	befriend("v", "lazy")
	befriend("v", "eu")
	befriend("v", "gone")
	befriend("undone", "v")
	for i, a := range others {
		for _, b := range others[i+1:] {
			if rng.IntN(100) < 2 {
				befriend(a, b)
			}
		}
	}

	follows := map[[2]string]bool{}
	follow := func(follower, followee string) {
		if follower == followee || follows[[2]string{follower, followee}] {
			return
		}
		follows[[2]string{follower, followee}] = true
		tx.MustExec(`INSERT INTO follows (id, follower_id, followee_id) VALUES ($1, $2, $3)`, follower+">"+followee, follower, followee)
	}
	for _, id := range pick(110) {
		follow("v", id)
	}
	follow("v", "star")
	follow("v", "gone")
	for _, id := range pick(20) {
		follow("eu", id)
	}
	for _, id := range pick(15) {
		follow("am", id)
	}
	for _, id := range pick(50) {
		follow(id, "star")
	}

	block := func(blocker, blocked string) {
		tx.MustExec(`INSERT INTO block_relations (id, blocker_id, blocked_id) VALUES ($1, $2, $3)`, blocker+"x"+blocked, blocker, blocked)
	}
	block("v", vFriends[0])
	block("v", vFriends[1])
	block(vFriends[2], "v")
	block("v", "star")
	for _, id := range pick(3) {
		block(id, "v")
	}
	for _, id := range pick(2) {
		block("eu", id)
	}

	postIDs := map[string]bool{}
	var livePosts []string
	visibilities := []string{"public", "friends", "friends_of_friends"}
	post := func(authorID, windowID, visibility string, deleted bool) {
		id := fmt.Sprintf("%08x-%04x", rng.Uint32(), rng.IntN(1<<16))
		if postIDs[id] {
			return
		}
		postIDs[id] = true
		var deletedAt *time.Time
		if deleted {
			deletedAt = &base
		} else {
			livePosts = append(livePosts, id)
		}
		// 秒単位にして同じ時刻の投稿を作り、(posted_at, id) の順を確かめる
		postedAt := base.Add(time.Duration(rng.IntN(40)) * time.Second)
		kind, latitude, longitude, locality := "none", (*float64)(nil), (*float64)(nil), (*string)(nil)
		switch rng.IntN(3) {
		case 1:
			lat, lng := float64(rng.IntN(180))-90, float64(rng.IntN(360))-180
			kind, latitude, longitude = "exact", &lat, &lng
		case 2:
			place := "Town " + authorID
			kind, locality = "approximate", &place
		}
		tx.MustExec(`INSERT INTO posts (id, author_id, posting_window_id, image_id, caption, visibility, location_kind, latitude, longitude, locality, posted_at, deleted_at)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			id, authorID, windowID, "image-"+id, "caption "+id, visibility, kind, latitude, longitude, locality, postedAt, deletedAt)
	}
	post("v", "w-asia-open", "public", false)
	post("lazy", "w-asia-ended", "public", false)
	post("undone", "w-asia-open", "public", true)
	post("eu", "w-europe-open", "friends", false)
	post("am", "w-america-open", "friends_of_friends", false)
	post("loner", "w-europe-open", "public", false)
	post("gone", "w-asia-open", "public", false)
	post("star", "w-asia-open", "public", false)
	for _, u := range users[len(special):] {
		visibility := func() string {
			// スキーマは公開範囲の値を縛らないので、アプリが使わない値も混ぜる
			if rng.IntN(100) == 0 {
				return "private"
			}
			return visibilities[rng.IntN(len(visibilities))]
		}
		if rng.IntN(100) < 85 {
			post(u.id, openWindow[u.region], visibility(), rng.IntN(100) < 10)
		}
		if rng.IntN(100) < 50 {
			post(u.id, endedWindow[u.region], visibility(), rng.IntN(100) < 10)
		}
		if rng.IntN(100) < 20 {
			post(u.id, openWindow[regions[rng.IntN(len(regions))]], visibility(), false)
		}
	}

	// recommendationScores の点数がばらつき、同点もできるように反応を付ける
	reacted := map[[2]string]bool{}
	for i := range 600 {
		postID := livePosts[rng.IntN(len(livePosts))]
		reactor := users[rng.IntN(len(users))].id
		if rng.IntN(2) == 0 {
			var deletedAt *time.Time
			if rng.IntN(10) == 0 {
				deletedAt = &base
			}
			tx.MustExec(`INSERT INTO comments (id, post_id, author_id, body, created_at, deleted_at) VALUES ($1, $2, $3, 'c', $4, $5)`,
				fmt.Sprintf("c%d", i), postID, reactor, base, deletedAt)
			continue
		}
		var replacedAt *time.Time
		if reacted[[2]string{postID, reactor}] || rng.IntN(10) == 0 {
			replacedAt = &base
		}
		reacted[[2]string{postID, reactor}] = true
		tx.MustExec(`INSERT INTO realmojis (id, post_id, reactor_id, image_id, reaction_type, created_at, replaced_at) VALUES ($1, $2, $3, 'r', 'smile', $4, $5)`,
			fmt.Sprintf("r%d", i), postID, reactor, base, replacedAt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return []string{"v", "lazy", "undone", "eu", "am", "loner", "gone", "star", others[0], others[150], others[299]}
}
