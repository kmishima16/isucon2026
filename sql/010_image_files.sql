-- 投稿の画像の INSERT が DB の時間の 1 位だった（58270 点の回で 1466 回・平均 166ms・合計 243 秒。1 枚約 380KB を 2KB ずつ TOAST に書く）。
-- 投稿の画像の中身は、投稿を受けた台のディスクに置き（webapp/go/image_store.go）、DB には中身を空にした行と、中身を持つ台（host）だけを書く。
-- host が空の画像（初期データ・プロフィール画像・RealMoji の画像）は、今までどおり DB に中身がある。設計は logs/notes/image-files-design.md。
-- 引数の数が違うので、sql/007 の関数とは別の関数として並ぶ（deploy の途中で古いアプリが呼んでも壊れない）。何度流しても壊れない
ALTER TABLE image_object_locations ADD COLUMN IF NOT EXISTS host text;
CREATE OR REPLACE FUNCTION isu_create_post(
  p_user_id varchar, p_post_id varchar, p_image_id varchar, p_data bytea,
  p_caption text, p_visibility text, p_location_kind text,
  p_latitude numeric, p_longitude numeric, p_locality text, p_posted_at timestamptz, p_host text)
RETURNS TABLE (
  o_id varchar, o_author_id varchar, o_posting_window_id varchar, o_image_id varchar, o_caption text,
  o_visibility text, o_location_kind text, o_latitude numeric, o_longitude numeric, o_locality text,
  o_posted_at timestamptz, o_on_time_deadline timestamptz, o_window_ended_at timestamptz,
  o_u_id varchar, o_u_username varchar, o_u_secret_key_hash text, o_u_display_name text, o_u_bio text,
  o_u_profile_image_id varchar, o_u_region text, o_u_account_type text, o_u_status text)
LANGUAGE plpgsql AS $$
DECLARE
  v_region text;
  v_window_id varchar;
  v_local_date date;
  v_starts_at timestamptz;
  v_active int;
  v_deleted int;
BEGIN
  PERFORM 1 FROM users u WHERE u.id = p_user_id FOR NO KEY UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'user not found';
  END IF;
  SELECT u.region INTO v_region FROM users u WHERE u.id = p_user_id AND u.status = 'active';
  IF NOT FOUND THEN
    RAISE EXCEPTION 'user not found';
  END IF;
  -- 投稿期間を閉じる UPDATE とだけ衝突すればよいので FOR SHARE。閉じる途中なら、待った後に次の期間を引き直す
  SELECT w.id, w.local_date, w.starts_at INTO v_window_id, v_local_date, v_starts_at
  FROM posting_windows w WHERE w.region = v_region AND w.ended_at IS NULL FOR SHARE;
  IF NOT FOUND THEN
    SELECT w.id, w.local_date, w.starts_at INTO v_window_id, v_local_date, v_starts_at
    FROM posting_windows w WHERE w.region = v_region AND w.ended_at IS NULL FOR SHARE;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'post_slot_unavailable' USING ERRCODE = 'IR409';
    END IF;
  END IF;
  -- 時刻はアプリが DB に送る前に決めている。待っている間に次の期間が始まると開始より前になり、409 を返していた（前の手順はロックの後に時刻を取っていた）。
  -- 開始の時刻も DB の時計で決まるので、取り直せば開始より後になる。画像・区画の表・投稿には同じ取り直した時刻を入れる
  IF p_posted_at < v_starts_at THEN
    p_posted_at := clock_timestamp();
  END IF;
  SELECT count(*) FILTER (WHERE p.deleted_at IS NULL), count(*) FILTER (WHERE p.deleted_at IS NOT NULL)
  INTO v_active, v_deleted
  FROM posts p JOIN posting_windows w ON w.id = p.posting_window_id
  WHERE p.author_id = p_user_id AND w.local_date = v_local_date;
  IF v_active <> 0 OR v_deleted >= 2 OR p_posted_at < v_starts_at THEN
    RAISE EXCEPTION 'post_slot_unavailable' USING ERRCODE = 'IR409';
  END IF;
  INSERT INTO image_objects (id, owner_user_id, data, media_type, created_at)
  VALUES (p_image_id, p_user_id, p_data, 'image/jpeg', p_posted_at);
  INSERT INTO image_object_locations (id, created_at, host) VALUES (p_image_id, p_posted_at, p_host);
  INSERT INTO posts (id, author_id, posting_window_id, image_id, caption, visibility,
                     location_kind, latitude, longitude, locality, posted_at)
  VALUES (p_post_id, p_user_id, v_window_id, p_image_id, p_caption, p_visibility,
          p_location_kind, p_latitude, p_longitude, p_locality, p_posted_at);
  RETURN QUERY
  SELECT p.id, p.author_id, p.posting_window_id, p.image_id, p.caption, p.visibility,
         p.location_kind, p.latitude, p.longitude, p.locality, p.posted_at,
         w.on_time_deadline, w.ended_at,
         u.id, u.username, u.secret_key_hash, u.display_name, u.bio,
         u.profile_image_id, u.region, u.account_type, u.status
  FROM posts p JOIN posting_windows w ON w.id = p.posting_window_id JOIN users u ON u.id = p.author_id
  WHERE p.id = p_post_id;
END
$$;
