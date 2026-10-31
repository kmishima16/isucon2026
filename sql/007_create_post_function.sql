-- 投稿の処理は、セッションの確認のほかに DB と約 9 往復していた（BEGIN・users のロック・getUser・期間・件数・画像・投稿・読み直し・COMMIT）。
-- europe・america の台は 1 往復 100ms かかり、投稿が 2 秒を超えて、新しく来た人が期間内に投稿できなかった（logs/notes/gap-to-first.md）。
-- 同じ手順を 1 つの関数にまとめ、1 往復で済ませる。手順と判定は webapp/go/post_api.go の前の実装と同じ。何度流しても壊れない
CREATE OR REPLACE FUNCTION isu_create_post(
  p_user_id varchar, p_post_id varchar, p_image_id varchar, p_data bytea,
  p_caption text, p_visibility text, p_location_kind text,
  p_latitude numeric, p_longitude numeric, p_locality text, p_posted_at timestamptz)
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
  SELECT count(*) FILTER (WHERE p.deleted_at IS NULL), count(*) FILTER (WHERE p.deleted_at IS NOT NULL)
  INTO v_active, v_deleted
  FROM posts p JOIN posting_windows w ON w.id = p.posting_window_id
  WHERE p.author_id = p_user_id AND w.local_date = v_local_date;
  IF v_active <> 0 OR v_deleted >= 2 OR p_posted_at < v_starts_at THEN
    RAISE EXCEPTION 'post_slot_unavailable' USING ERRCODE = 'IR409';
  END IF;
  INSERT INTO image_objects (id, owner_user_id, data, media_type, created_at)
  VALUES (p_image_id, p_user_id, p_data, 'image/jpeg', p_posted_at);
  INSERT INTO image_object_locations (id, created_at) VALUES (p_image_id, p_posted_at);
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
