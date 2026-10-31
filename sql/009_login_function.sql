-- ログインは DB と約 7 往復していた（BEGIN・利用者のロック・もう一度ロック・前のセッションの無効化・セッションの INSERT・読み直し・COMMIT）。
-- europe・america では 1 往復 100ms かかり、新しく来た人の経路を遅くしていた（logs/notes/gap-to-first.md）。
-- 同じ手順を 1 つの関数にまとめ、1 往復で済ませる。鍵が合わないときと退会済みのときは何も書かずに 0 行を返す（401）。何度流しても壊れない
CREATE OR REPLACE FUNCTION isu_login(p_username varchar, p_secret_key_hash text, p_session_id varchar, p_token_hash text)
RETURNS TABLE (
  o_id varchar, o_username varchar, o_secret_key_hash text, o_display_name text, o_bio text,
  o_profile_image_id varchar, o_region text, o_account_type text, o_status text)
LANGUAGE plpgsql AS $$
DECLARE
  v_user users%ROWTYPE;
BEGIN
  SELECT * INTO v_user FROM users u WHERE u.username = p_username FOR NO KEY UPDATE;
  IF NOT FOUND OR v_user.status <> 'active' OR v_user.secret_key_hash <> p_secret_key_hash THEN
    RETURN;
  END IF;
  UPDATE login_sessions SET active = false, invalidated_at = clock_timestamp(), reason = 'replaced'
  WHERE user_id = v_user.id AND active = true;
  INSERT INTO login_sessions (id, user_id, token_hash) VALUES (p_session_id, v_user.id, p_token_hash);
  RETURN QUERY SELECT v_user.id, v_user.username, v_user.secret_key_hash, v_user.display_name, v_user.bio,
    v_user.profile_image_id, v_user.region, v_user.account_type, v_user.status;
END
$$;
