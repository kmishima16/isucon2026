package isureal

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
)

// DB（asia2）まで 1 往復 100ms の europe1・america1 では、loadViewerContext の 5 回のクエリ（閲覧者・今日投稿したか・ブロック・友達・友達の友達）が
// 友達フィードを 0.5 秒遅らせ、新しい人の投稿の後の検証が期間の終わりに間に合わなかった。1 回のクエリにまとめる。
// asia1・asia2 は往復 0.4ms で得が無く、まとめた SQL の重さだけが DB に乗るので、今のまま 5 回で読む（ISUREAL_VIEWER_ONE_QUERY=1 の台だけ）
var viewerOneQuery = os.Getenv("ISUREAL_VIEWER_ONE_QUERY") == "1"

// 友達どうしの組は、友達を片方に持つ行を、低い側と高い側で別々に引いて UNION する。
// 「low IN (副問い合わせ) OR high IN (副問い合わせ)」の形は friendships を全件なめ、友達の多い人で 1 回 117ms かかって asia2 の CPU を使い切った（e0cabdd、57161 点）。
// 組は一意（user_low_id, user_high_id）なので、UNION で重なりを除いても、前の 1 本の SELECT と同じ行になる。
// id は UUID で ',' と ' ' を含まないので、区切り文字でまとめて受け取る
const viewerOneQuerySQL = `
  SELECT %s,
    EXISTS(
      SELECT 1 FROM posts p
      JOIN posting_windows posted ON posted.id = p.posting_window_id
      JOIN posting_windows current ON current.region = u.region AND current.ended_at IS NULL
                                  AND current.local_date = posted.local_date
      WHERE p.author_id = u.id AND p.deleted_at IS NULL
    ) AS has_posted_today,
    (SELECT coalesce(string_agg(CASE WHEN blocker_id = $1 THEN blocked_id ELSE blocker_id END, ','), '')
     FROM block_relations WHERE blocker_id = $1 OR blocked_id = $1) AS blocked_ids,
    (WITH fr AS (
       SELECT CASE WHEN user_low_id = $1 THEN user_high_id ELSE user_low_id END AS id
       FROM friendships WHERE user_low_id = $1 OR user_high_id = $1
     )
     SELECT coalesce(string_agg(pairs.user_low_id || ' ' || pairs.user_high_id, ','), '')
     FROM (
       SELECT fs.user_low_id, fs.user_high_id FROM fr JOIN friendships fs ON fs.user_low_id = fr.id
       UNION
       SELECT fs.user_low_id, fs.user_high_id FROM fr JOIN friendships fs ON fs.user_high_id = fr.id
     ) pairs) AS friend_pairs
  FROM users u WHERE u.id = $1 AND u.status = 'active'
`

var viewerOneQueryStatement = fmt.Sprintf(viewerOneQuerySQL, strings.Join(prefixedColumns("u", userColumns), ", "))

func loadViewerContextOneQuery(ctx context.Context, db sqlx.QueryerContext, viewerID string) (viewerContext, error) {
	viewer := viewerContext{Friends: map[string]bool{}, FriendsOfFriends: map[string]bool{}, MutualFriends: map[string]int{}}
	var row struct {
		User
		HasPostedToday bool   `db:"has_posted_today"`
		BlockedIDs     string `db:"blocked_ids"`
		FriendPairs    string `db:"friend_pairs"`
	}
	if err := sqlx.GetContext(ctx, db, &row, viewerOneQueryStatement, viewerID); err != nil {
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

func prefixedColumns(alias string, columns []string) []string {
	out := make([]string, len(columns))
	for i, column := range columns {
		out[i] = alias + "." + column
	}
	return out
}
