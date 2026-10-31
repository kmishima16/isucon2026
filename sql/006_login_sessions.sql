-- セッションの確認は全リクエストで token_hash を引き、ログインは user_id で前のセッションを無効にする。
-- login_sessions には主キーしか無く、ログインが増えるほど全リクエストが表を頭から読んでいた（1 回あたり約 48 ブロック）。
-- 初期化（reset.sql）は TRUNCATE するだけでインデックスは残る。何度流しても壊れない
CREATE INDEX IF NOT EXISTS login_sessions_token_hash ON login_sessions(token_hash);
CREATE INDEX IF NOT EXISTS login_sessions_user_id_active ON login_sessions(user_id) WHERE active;
