-- ユーザー検索は、全員分の username と display_name を毎回 1 つの文字列にまとめて読み、アプリで照合していた（1 回 57ms、DB の時間の 2 位）。
-- アプリの正規化（normalizeSearchText）の結果を列に持ち、DB で絞ってから読む。正規化は DB の関数と一致しない文字があるので、値はアプリが書く。
-- NULL の行は必ず候補に入れてアプリで確かめるので、書き漏れがあっても遅くなるだけで結果は変わらない。何度流しても壊れない
ALTER TABLE users ADD COLUMN IF NOT EXISTS search_username text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS search_display_name text;

-- 名前が変わったのに正規化を書き直さなかった更新（reset.sql の復元など）で、古い値が残らないようにする
CREATE OR REPLACE FUNCTION isu_users_search_text_invalidate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.username IS DISTINCT FROM OLD.username AND NEW.search_username IS NOT DISTINCT FROM OLD.search_username THEN
    NEW.search_username := NULL;
  END IF;
  IF NEW.display_name IS DISTINCT FROM OLD.display_name AND NEW.search_display_name IS NOT DISTINCT FROM OLD.search_display_name THEN
    NEW.search_display_name := NULL;
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS users_search_text_invalidate ON users;
CREATE TRIGGER users_search_text_invalidate BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION isu_users_search_text_invalidate();
-- reset.sql は session_replication_role = replica で流れ、普通のトリガーは動かない。表示名を初期値に戻しても古い正規化が残り、次の走行の検索から外れるので、replica でも動かす
ALTER TABLE users ENABLE ALWAYS TRIGGER users_search_text_invalidate;

-- 照合に使う列と、結果を返す列を username, id の順に持ち、並べ替えなしに先頭から読む
CREATE INDEX IF NOT EXISTS users_active_username_id_search ON users(username, id)
  INCLUDE (search_username, search_display_name) WHERE status = 'active';
DROP INDEX IF EXISTS users_active_username_id;
