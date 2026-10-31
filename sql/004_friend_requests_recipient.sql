-- 受け取った友達申請の一覧（recipient_id = $1 AND status = 'pending'）が、毎回 friend_requests を全件走査していた
-- 既存の (requester_id, recipient_id) では recipient_id から引けない
-- 初期化（reset.sql）は friend_requests を TRUNCATE するだけで作り直さないので、ここに置くだけで残る。何度流しても壊れない
CREATE INDEX IF NOT EXISTS friend_requests_recipient_status ON friend_requests(recipient_id, status);
