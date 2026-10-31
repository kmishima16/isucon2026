-- ユーザー検索は、利用者の写しを asia の台のメモリに持ち、検索のたびに「写しより後に変わった行」だけを DB から読む（webapp/go/search_snapshot.go）。
-- その差分を updated_at で引くためのインデックスと、初期化のたびに増やす世代（初期化は updated_at を古い値に戻すので、差分では気付けない）。
-- 何度流しても壊れない
CREATE INDEX IF NOT EXISTS users_updated_at ON users(updated_at);
CREATE TABLE IF NOT EXISTS app_generation (id int PRIMARY KEY, n bigint NOT NULL);
INSERT INTO app_generation (id, n) VALUES (1, 0) ON CONFLICT (id) DO NOTHING;
