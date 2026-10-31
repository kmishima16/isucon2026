-- 友達申請の承認・拒否・取り下げとフォローは、DB と 6〜8 往復していた（申請の読み取り・BEGIN・2 人の組のロック・状態の読み直し・ブロック・書き込み・読み直し・COMMIT）。
-- europe1 では承認が 1.19 秒・フォローが 1.22 秒（asia1 は 0.43 秒）かかり、新しく来た人が投稿期間の 9 秒のうちに投稿を終えられず、翌日の新しい人が増えなかった。
-- 同じ手順を 1 つの関数にまとめ、1 往復で済ませる。手順と判定と順番は webapp/go の前の実装と同じ。何度流しても壊れない
CREATE OR REPLACE FUNCTION isu_transition_friend_request(p_actor varchar, p_request_id varchar, p_status text)
RETURNS TABLE (o_id varchar, o_status text, o_updated_at timestamptz)
LANGUAGE plpgsql AS $$
DECLARE
  v_requester varchar;
  v_recipient varchar;
  v_low varchar;
  v_high varchar;
  v_active int;
  v_current text;
BEGIN
  SELECT r.requester_id, r.recipient_id INTO v_requester, v_recipient FROM friend_requests r WHERE r.id = p_request_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'relationship_not_found' USING ERRCODE = 'IR404';
  END IF;
  IF (p_status = 'cancelled' AND p_actor <> v_requester) OR (p_status <> 'cancelled' AND p_actor <> v_recipient) THEN
    RAISE EXCEPTION 'relationship_action_forbidden' USING ERRCODE = 'IR403';
  END IF;
  v_low := least(v_requester, v_recipient);
  v_high := greatest(v_requester, v_recipient);
  -- 同じ 2 人の間の操作だけを並べる（webapp/go/relationship.go の beginRelationshipTx と同じ鍵）
  PERFORM pg_advisory_xact_lock(hashtext(v_low || ':' || v_high));
  SELECT count(*) INTO v_active FROM users u WHERE u.id IN (v_low, v_high) AND u.status = 'active';
  IF v_active <> 2 THEN
    RAISE EXCEPTION 'relationship_not_found' USING ERRCODE = 'IR404';
  END IF;
  SELECT r.status INTO v_current FROM friend_requests r WHERE r.id = p_request_id;
  IF v_current <> 'pending' THEN
    RAISE EXCEPTION 'relationship_action_forbidden' USING ERRCODE = 'IR403';
  END IF;
  IF p_status = 'accepted' THEN
    IF EXISTS (SELECT 1 FROM block_relations b WHERE (b.blocker_id = v_requester AND b.blocked_id = v_recipient) OR (b.blocker_id = v_recipient AND b.blocked_id = v_requester)) THEN
      RAISE EXCEPTION 'relationship_conflict' USING ERRCODE = 'IR409';
    END IF;
    INSERT INTO friendships (id, user_low_id, user_high_id, source_request_id) VALUES (gen_random_uuid()::text, v_low, v_high, p_request_id);
  END IF;
  UPDATE friend_requests SET status = p_status, updated_at = clock_timestamp() WHERE id = p_request_id;
  RETURN QUERY SELECT r.id, r.status, r.updated_at FROM friend_requests r WHERE r.id = p_request_id;
END
$$;

CREATE OR REPLACE FUNCTION isu_follow(p_actor varchar, p_target varchar, p_new_id varchar)
RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  v_low varchar := least(p_actor, p_target);
  v_high varchar := greatest(p_actor, p_target);
  v_active int;
  v_account_type text;
BEGIN
  PERFORM pg_advisory_xact_lock(hashtext(v_low || ':' || v_high));
  SELECT count(*) INTO v_active FROM users u WHERE u.id IN (v_low, v_high) AND u.status = 'active';
  IF v_active <> 2 THEN
    RAISE EXCEPTION 'relationship_not_found' USING ERRCODE = 'IR404';
  END IF;
  SELECT u.account_type INTO v_account_type FROM users u WHERE u.id = p_target;
  IF v_account_type <> 'certified_public' THEN
    RAISE EXCEPTION 'relationship_conflict' USING ERRCODE = 'IR409';
  END IF;
  IF EXISTS (SELECT 1 FROM block_relations b WHERE (b.blocker_id = p_actor AND b.blocked_id = p_target) OR (b.blocker_id = p_target AND b.blocked_id = p_actor)) THEN
    RAISE EXCEPTION 'relationship_conflict' USING ERRCODE = 'IR409';
  END IF;
  IF EXISTS (SELECT 1 FROM follows f WHERE f.follower_id = p_actor AND f.followee_id = p_target) THEN
    RAISE EXCEPTION 'relationship_conflict' USING ERRCODE = 'IR409';
  END IF;
  INSERT INTO follows (id, follower_id, followee_id) VALUES (p_new_id, p_actor, p_target);
END
$$;
