-- 初期化（reset.sql）はスキーマを作り直さないので、ここに置くだけで残る。何度流しても壊れない
-- フィードは開いている投稿期間の投稿を投稿者で絞る。posting_window_id だけでは期間の投稿を全部読み、author_id だけでは投稿者の過去の投稿まで全部読む
-- pgx は文を使い回すので、6 回目からは汎用プランになり、author_id で過去の投稿まで読むプランを選んでいた
CREATE INDEX IF NOT EXISTS posts_posting_window_id_author_id ON posts(posting_window_id, author_id);
