package isureal

import (
	"context"
	"database/sql"
	"errors"
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

type imageOwnerKey struct {
	SecretKeyHash string    `db:"secret_key_hash"`
	CreatedAt     time.Time `db:"created_at"`
	// 中身を持つ台の内部の窓口。空なら中身は DB にある（image_store.go）
	Host *string `db:"host"`
}

// 持ち主が active な画像の、持ち主の鍵と区画の時刻を返す
func getImageOwnerKey(ctx context.Context, db sqlx.QueryerContext, imageID string) (imageOwnerKey, error) {
	var row imageOwnerKey
	err := sqlx.GetContext(ctx, db, &row, `
  SELECT u.secret_key_hash, i.created_at, (SELECT host FROM image_object_locations WHERE id = $1) AS host
  FROM image_objects i JOIN users u ON u.id = i.owner_user_id
  WHERE i.id = $1 AND u.status = 'active'
    AND i.created_at = (SELECT created_at FROM image_object_locations WHERE id = $1)
 `, imageID)
	if !errors.Is(err, sql.ErrNoRows) {
		return row, err
	}
	err = sqlx.GetContext(ctx, db, &row, `
  SELECT u.secret_key_hash, i.created_at, NULL::text AS host
  FROM image_objects i JOIN users u ON u.id = i.owner_user_id
  WHERE i.id = $1 AND u.status = 'active'
 `, imageID)
	return row, err
}
