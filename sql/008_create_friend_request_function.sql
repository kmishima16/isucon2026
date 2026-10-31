-- 友達申請は、セッションの確認のほかに DB と約 9 往復していた（BEGIN・2 人の組のロック・ブロック・友達・申請の確認・申請の書き込み・読み直し・COMMIT）。
-- 新しく来た人は投稿の前に友達申請を 5 回送るので、europe・america では申請だけで数秒かかっていた（logs/notes/gap-to-first.md）。
-- 同じ手順を 1 つの関数にまとめ、1 往復で済ませる。手順と判定は webapp/go/friend_request_api.go の前の実装と同じ。何度流しても壊れない
CREATE OR REPLACE FUNCTION isu_create_friend_request(p_actor varchar, p_recipient varchar, p_new_id varchar)
RETURNS TABLE (o_id varchar, o_status text, o_updated_at timestamptz)
LANGUAGE plpgsql AS $$
DECLARE
  v_low varchar := least(p_actor, p_recipient);
  v_high varchar := greatest(p_actor, p_recipient);
  v_active int;
  v_existing_id varchar;
  v_existing_requester varchar;
  v_request_id varchar;
BEGIN
  -- 同じ 2 人の間の操作だけを並べる（webapp/go/relationship.go の beginRelationshipTx と同じ鍵）
  PERFORM pg_advisory_xact_lock(hashtext(v_low || ':' || v_high));
  SELECT count(*) INTO v_active FROM users u WHERE u.id IN (v_low, v_high) AND u.status = 'active';
  IF v_active <> 2 THEN
    RAISE EXCEPTION 'relationship_not_found' USING ERRCODE = 'IR404';
  END IF;
  IF EXISTS (SELECT 1 FROM block_relations b WHERE (b.blocker_id = p_actor AND b.blocked_id = p_recipient) OR (b.blocker_id = p_recipient AND b.blocked_id = p_actor)) THEN
    RAISE EXCEPTION 'relationship_conflict' USING ERRCODE = 'IR409';
  END IF;
  IF EXISTS (SELECT 1 FROM friendships f WHERE f.user_low_id = v_low AND f.user_high_id = v_high) THEN
    RAISE EXCEPTION 'relationship_conflict' USING ERRCODE = 'IR409';
  END IF;
  SELECT r.id, r.requester_id INTO v_existing_id, v_existing_requester FROM friend_requests r
  WHERE r.status = 'pending' AND ((r.requester_id = p_actor AND r.recipient_id = p_recipient) OR (r.requester_id = p_recipient AND r.recipient_id = p_actor))
  LIMIT 1;
  IF FOUND THEN
    IF v_existing_requester = p_actor THEN
      RAISE EXCEPTION 'relationship_conflict' USING ERRCODE = 'IR409';
    END IF;
    UPDATE friend_requests SET status = 'accepted', updated_at = clock_timestamp() WHERE id = v_existing_id;
    INSERT INTO friendships (id, user_low_id, user_high_id, source_request_id) VALUES (gen_random_uuid()::text, v_low, v_high, v_existing_id);
    v_request_id := v_existing_id;
  ELSE
    INSERT INTO friend_requests (id, requester_id, recipient_id, status, created_at, updated_at)
    VALUES (p_new_id, p_actor, p_recipient, 'pending', clock_timestamp(), clock_timestamp());
    v_request_id := p_new_id;
  END IF;
  RETURN QUERY SELECT r.id, r.status, r.updated_at FROM friend_requests r WHERE r.id = v_request_id;
END
$$;
