-- 初期化（reset.sql）はスキーマを作り直さないので、ここに置くだけで残る。何度流しても壊れない
-- GET /api/images の権限確認が、RealMoji の画像のたびに realmojis（120 万行）を全件走査していた
CREATE INDEX IF NOT EXISTS realmojis_image_id ON realmojis(image_id);
-- 同じ権限確認で、投稿の画像を image_id で引く
CREATE INDEX IF NOT EXISTS posts_image_id ON posts(image_id);
-- 友達の取得は user_low_id = $1 OR user_high_id = $1。UNIQUE(user_low_id, user_high_id) では後ろ半分を引けない
CREATE INDEX IF NOT EXISTS friendships_user_high_id ON friendships(user_high_id);
-- 友達申請の重複確認（requester と recipient を入れ替えた OR）
CREATE INDEX IF NOT EXISTS friend_requests_requester_recipient ON friend_requests(requester_id, recipient_id);
