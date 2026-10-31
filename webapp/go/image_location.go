package isureal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// image_objects は created_at の月で区画を分けてあり、id だけで引くと約 36 の区画をすべて引く（1 回あたり約 345 バッファ）。
// id から created_at を引く表 image_object_locations（sql/005_image_object_locations.sql）を画像と同じ文で書き、引くときは区画を 1 つに絞る。
// 表に無い画像（deploy で sql を流してからアプリが入れ替わるまでに古いアプリが書いたもの）は、id だけで引き直す

func insertImageObject(ctx context.Context, tx *sqlx.Tx, imageID, ownerID string, data []byte, createdAt *time.Time) error {
	_, err := tx.ExecContext(ctx, `
  WITH i AS (
    INSERT INTO image_objects (id, owner_user_id, data, media_type, created_at)
    VALUES ($1, $2, $3, 'image/jpeg', COALESCE($4::timestamptz, clock_timestamp()))
    RETURNING id, created_at
  )
  INSERT INTO image_object_locations (id, created_at) SELECT id, created_at FROM i
 `, imageID, ownerID, data, createdAt)
	return err
}

func deleteImageObject(ctx context.Context, tx *sqlx.Tx, imageID string) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM image_objects WHERE id = $1 AND created_at = (SELECT created_at FROM image_object_locations WHERE id = $1)`, imageID)
	if err != nil {
		return err
	}
	if deleted, err := result.RowsAffected(); err != nil || deleted > 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM image_objects WHERE id = $1`, imageID)
	return err
}

// 画像を返してよいかの判定に要るものを、1 回のクエリでまとめて読む。
// 前は持ち主の鍵を読むクエリと生死確認のクエリの 2 回だった（DB の時間の 1 位と 4 位、画像の取得 1 回で 2 往復）。
// 鍵を台のメモリに覚える形は、画像の大半が 1 台で 1 回しか取られず、当たりが約 8% で効かなかった
type servableImage struct {
	SecretKeyHash string    `db:"secret_key_hash"`
	CreatedAt     time.Time `db:"created_at"`
	// 中身を持つ台の内部の窓口。空なら中身は DB にある（image_store.go）
	Host      *string `db:"host"`
	MediaType string  `db:"media_type"`
	// 持ち主のプロフィール画像か、消されていない投稿の画像か、生きている RealMoji の画像か（今の SQL と同じ条件）
	Live bool   `db:"live"`
	Data []byte `db:"data"`
}

const servableImageColumns = `u.secret_key_hash, i.created_at, %s AS host, i.media_type,
    (COALESCE(u.profile_image_id = i.id, false)
     OR EXISTS (
       SELECT 1 FROM posts p
       WHERE p.image_id = i.id AND p.author_id = u.id AND p.deleted_at IS NULL
     )
     OR EXISTS (
       SELECT 1 FROM realmojis r
       JOIN posts p ON p.id = r.post_id AND p.deleted_at IS NULL
       JOIN users author ON author.id = p.author_id AND author.status = 'active'
       WHERE r.image_id = i.id AND r.reactor_id = u.id AND r.replaced_at IS NULL AND r.deleted_at IS NULL
     )) AS live,
    CASE WHEN $2 OR %s IS NOT NULL THEN NULL ELSE i.data END AS data`

var (
	servableImageLocated = `
  SELECT ` + fmt.Sprintf(servableImageColumns, "l.host", "l.host") + `
  FROM image_object_locations l
  JOIN image_objects i ON i.id = l.id AND i.created_at = l.created_at
  JOIN users u ON u.id = i.owner_user_id
  WHERE l.id = $1 AND u.status = 'active'`
	servableImageLegacy = `
  SELECT ` + fmt.Sprintf(servableImageColumns, "NULL::text", "NULL::text") + `
  FROM image_objects i JOIN users u ON u.id = i.owner_user_id
  WHERE i.id = $1 AND u.status = 'active'`
)

// getServableImage は、持ち主が active な画像の鍵・区画の時刻・置き場所・種類・生死と、要るなら中身を返す。
// skipData なら中身は読まない（この台が持っている）。中身を持つ台が別にあるときも読まない
func getServableImage(ctx context.Context, db sqlx.QueryerContext, imageID string, skipData bool) (servableImage, error) {
	var row servableImage
	err := sqlx.GetContext(ctx, db, &row, servableImageLocated, imageID, skipData)
	if !errors.Is(err, sql.ErrNoRows) {
		return row, err
	}
	err = sqlx.GetContext(ctx, db, &row, servableImageLegacy, imageID, skipData)
	return row, err
}
