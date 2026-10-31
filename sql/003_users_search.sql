-- ユーザー検索は有効なユーザー全員を username, id の順に読んで、毎回 Seq Scan と Sort をしていた（1 回 82ms）
-- 照合に使う 3 列だけをこの順に持ち、Index Only Scan で並べ替えなしに読む
-- 初期化（reset.sql）は users を作り直さないので、ここに置くだけで残る。何度流しても壊れない
CREATE INDEX IF NOT EXISTS users_active_username_id ON users(username, id) INCLUDE (display_name) WHERE status = 'active';
