-- 登録はユーザー名の重複を表ロックで防いでいて、登録どうしと users の更新を待たせていた。一意インデックスで防ぐ（初期データに重複は無い）
-- 初期化（reset.sql）は users を作り直さないので、ここに置くだけで残る。何度流しても壊れない
CREATE UNIQUE INDEX IF NOT EXISTS users_username ON users(username);
