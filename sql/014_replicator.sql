-- europe1・america1 に asia2 の複製（hot standby）を置き、europe・america の読み取りを台の中で済ませるための、複製用の利用者。
-- 何度流しても壊れないよう、無いときだけ作る
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'replicator') THEN
    CREATE ROLE replicator WITH REPLICATION LOGIN PASSWORD 'replicator';
  END IF;
END
$$;
