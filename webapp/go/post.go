package isureal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type PostLocation struct {
	Kind      string   `json:"kind" db:"location_kind"`
	Latitude  *float64 `json:"latitude,omitempty" db:"latitude"`
	Longitude *float64 `json:"longitude,omitempty" db:"longitude"`
	Locality  *string  `json:"locality,omitempty" db:"locality"`
}

func (location PostLocation) valid() bool {
	switch location.Kind {
	case "none":
		return location.Latitude == nil && location.Longitude == nil && location.Locality == nil
	case "exact":
		return location.Latitude != nil && location.Longitude != nil && location.Locality == nil &&
			*location.Latitude >= -90 && *location.Latitude <= 90 && *location.Longitude >= -180 && *location.Longitude <= 180
	case "approximate":
		return location.Latitude == nil && location.Longitude == nil && location.Locality != nil && validUserText(*location.Locality, 1, 100)
	default:
		return false
	}
}

type Post struct {
	ID              string `db:"id"`
	AuthorID        string `db:"author_id"`
	PostingWindowID string `db:"posting_window_id"`
	ImageID         string `db:"image_id"`
	Caption         string `db:"caption"`
	Visibility      string `db:"visibility"`
	PostLocation
	PostedAt       time.Time  `db:"posted_at"`
	OnTimeDeadline time.Time  `db:"on_time_deadline"`
	WindowEndedAt  *time.Time `db:"window_ended_at"`
}

const postColumns = `p.id, p.author_id, p.posting_window_id, p.image_id, p.caption, p.visibility,
  p.location_kind, p.latitude, p.longitude, p.locality, p.posted_at,
  w.on_time_deadline, w.ended_at AS window_ended_at`

func getPost(ctx context.Context, db sqlx.QueryerContext, postID string) (Post, error) {
	var post Post
	err := sqlx.GetContext(ctx, db, &post, `
  SELECT `+postColumns+`
  FROM posts p JOIN posting_windows w ON w.id = p.posting_window_id
  WHERE p.id = $1 AND p.deleted_at IS NULL
 `, postID)
	return post, err
}

type PostWithAuthor struct {
	Post
	Author User `db:"author"`
}

func selectPostsWithAuthor(ctx context.Context, db sqlx.QueryerContext, conditions string, args ...any) ([]PostWithAuthor, error) {
	var posts []PostWithAuthor
	if err := sqlx.SelectContext(ctx, db, &posts, `
  SELECT `+postColumns+`, `+userColumnsAs("u", "author")+`
  FROM posts p
  JOIN posting_windows w ON w.id = p.posting_window_id
  JOIN users u ON u.id = p.author_id AND u.status = 'active'
  WHERE p.deleted_at IS NULL `+conditions, args...); err != nil {
		return nil, err
	}
	for i := range posts {
		if err := posts[i].Author.setImageKey(); err != nil {
			return nil, err
		}
	}
	return posts, nil
}

func getPostWithAuthor(ctx context.Context, db sqlx.QueryerContext, postID string) (PostWithAuthor, error) {
	posts, err := selectPostsWithAuthor(ctx, db, `AND p.id = $1`, postID)
	if err != nil {
		return PostWithAuthor{}, err
	}
	if len(posts) == 0 {
		return PostWithAuthor{}, sql.ErrNoRows
	}
	return posts[0], nil
}

type viewerContext struct {
	User
	HasPostedToday   bool
	Friends          map[string]bool
	FriendsOfFriends map[string]bool
	MutualFriends    map[string]int
	Blocked          map[string]bool
}

// 閲覧者の行・今日投稿したか・ブロック・友達と友達の友達を 1 回のクエリで読む。
// 前は 5 回の往復で、DB まで 1 往復 100ms の europe1・america1 では、新しい人が投稿の後に見る友達フィードが期間の終わりに間に合わなかった。
// 友達どうしの組は「友達が片方にいる行」を全部読む（閲覧者自身の行も入る）ので、友達はその中の閲覧者を含む行から取る。
// id は UUID で ',' と ' ' を含まないので、区切り文字でまとめて受け取る
func loadViewerContext(ctx context.Context, db sqlx.QueryerContext, viewerID string) (viewerContext, error) {
	viewer := viewerContext{Friends: map[string]bool{}, FriendsOfFriends: map[string]bool{}, MutualFriends: map[string]int{}}
	var row struct {
		User
		HasPostedToday bool   `db:"has_posted_today"`
		BlockedIDs     string `db:"blocked_ids"`
		FriendPairs    string `db:"friend_pairs"`
	}
	if err := sqlx.GetContext(ctx, db, &row, `
  SELECT `+strings.Join(prefixed("u", userColumns), ", ")+`,
    EXISTS(
      SELECT 1 FROM posts p
      JOIN posting_windows posted ON posted.id = p.posting_window_id
      JOIN posting_windows current ON current.region = u.region AND current.ended_at IS NULL
                                  AND current.local_date = posted.local_date
      WHERE p.author_id = u.id AND p.deleted_at IS NULL
    ) AS has_posted_today,
    (SELECT coalesce(string_agg(CASE WHEN blocker_id = $1 THEN blocked_id ELSE blocker_id END, ','), '')
     FROM block_relations WHERE blocker_id = $1 OR blocked_id = $1) AS blocked_ids,
    (SELECT coalesce(string_agg(fs.user_low_id || ' ' || fs.user_high_id, ','), '')
     FROM friendships fs
     WHERE fs.user_low_id IN (SELECT CASE WHEN user_low_id = $1 THEN user_high_id ELSE user_low_id END FROM friendships WHERE user_low_id = $1 OR user_high_id = $1)
        OR fs.user_high_id IN (SELECT CASE WHEN user_low_id = $1 THEN user_high_id ELSE user_low_id END FROM friendships WHERE user_low_id = $1 OR user_high_id = $1)) AS friend_pairs
  FROM users u WHERE u.id = $1 AND u.status = 'active'
 `, viewerID); err != nil {
		return viewer, err
	}
	viewer.User = row.User
	if err := viewer.User.setImageKey(); err != nil {
		return viewer, err
	}
	viewer.HasPostedToday = row.HasPostedToday
	viewer.Blocked = map[string]bool{}
	if row.BlockedIDs != "" {
		for _, id := range strings.Split(row.BlockedIDs, ",") {
			viewer.Blocked[id] = true
		}
	}
	if row.FriendPairs == "" {
		return viewer, nil
	}
	type pair struct{ low, high string }
	pairs := make([]pair, 0, strings.Count(row.FriendPairs, ",")+1)
	for _, item := range strings.Split(row.FriendPairs, ",") {
		low, high, ok := strings.Cut(item, " ")
		if !ok {
			return viewer, fmt.Errorf("malformed friend pair %q", item)
		}
		pairs = append(pairs, pair{low, high})
		if low == viewer.ID {
			viewer.Friends[high] = true
		} else if high == viewer.ID {
			viewer.Friends[low] = true
		}
	}
	for _, friendship := range pairs {
		if viewer.Friends[friendship.low] {
			viewer.FriendsOfFriends[friendship.high] = true
			if friendship.high != viewer.ID {
				viewer.MutualFriends[friendship.high]++
			}
		}
		if viewer.Friends[friendship.high] {
			viewer.FriendsOfFriends[friendship.low] = true
			if friendship.low != viewer.ID {
				viewer.MutualFriends[friendship.low]++
			}
		}
	}
	return viewer, nil
}

func prefixed(alias string, columns []string) []string {
	out := make([]string, len(columns))
	for i, column := range columns {
		out[i] = alias + "." + column
	}
	return out
}

func (viewer viewerContext) canView(post Post) bool {
	if post.AuthorID == viewer.ID {
		return true
	}
	if post.WindowEndedAt != nil || !viewer.HasPostedToday || viewer.Blocked[post.AuthorID] {
		return false
	}
	switch post.Visibility {
	case "public":
		return true
	case "friends":
		return viewer.Friends[post.AuthorID]
	case "friends_of_friends":
		return viewer.Friends[post.AuthorID] || viewer.FriendsOfFriends[post.AuthorID]
	default:
		return false
	}
}

func canViewPost(ctx context.Context, db sqlx.QueryerContext, viewerID, postID string) (bool, error) {
	post, err := getPostWithAuthor(ctx, db, postID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	viewer, err := loadViewerContext(ctx, db, viewerID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return viewer.canView(post.Post), nil
}

func getFriendIDs(ctx context.Context, db sqlx.QueryerContext, userID string) ([]string, error) {
	var friendships []struct {
		UserLowID  string `db:"user_low_id"`
		UserHighID string `db:"user_high_id"`
	}
	if err := sqlx.SelectContext(ctx, db, &friendships, `SELECT user_low_id, user_high_id FROM friendships WHERE user_low_id = $1 OR user_high_id = $1`, userID); err != nil {
		return nil, err
	}
	var ids []string
	for _, friendship := range friendships {
		if friendship.UserLowID == userID {
			ids = append(ids, friendship.UserHighID)
		} else {
			ids = append(ids, friendship.UserLowID)
		}
	}
	return ids, nil
}

func getBlockedUserIDs(ctx context.Context, db sqlx.QueryerContext, userID string) (map[string]bool, error) {
	var ids []string
	if err := sqlx.SelectContext(ctx, db, &ids, `
  SELECT CASE WHEN blocker_id = $1 THEN blocked_id ELSE blocker_id END
  FROM block_relations WHERE blocker_id = $1 OR blocked_id = $1
 `, userID); err != nil {
		return nil, err
	}
	blocked := make(map[string]bool, len(ids))
	for _, id := range ids {
		blocked[id] = true
	}
	return blocked, nil
}
