-- 画像の取得は image_objects を id だけで引いていて、created_at の月の区画（約 36）をすべて引いていた（1 回あたり約 345 バッファ）。
-- id から created_at を引く表を足し、区画を 1 つに絞る。新しい画像はアプリが画像と同じ文で書く（webapp/go/image_location.go）
-- 初期化（reset.sql）は消した画像の行を残すが、引いた先に画像が無いだけで害は無い
CREATE TABLE IF NOT EXISTS image_object_locations (
  id varchar(36) PRIMARY KEY,
  created_at timestamptz NOT NULL
);
-- 既存の画像（約 183 万行）を埋める。2 回目からは、埋めた中で一番新しい時刻の 1 日前より後の区画だけを読むので速く終わる。
-- sql を流してからアプリが入れ替わるまでに古いアプリが書いた画像も、次の deploy で埋まる（埋まるまではアプリが id だけで引き直す）
INSERT INTO image_object_locations (id, created_at)
SELECT id, created_at FROM image_objects
WHERE created_at >= COALESCE((SELECT max(created_at) FROM image_object_locations) - interval '1 day', '-infinity')
ON CONFLICT (id) DO NOTHING;
