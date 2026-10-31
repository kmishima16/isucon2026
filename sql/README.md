# sql/

DB のスキーマ変更（インデックス追加など）を `NNN_説明.sql` で置く。`make deploy` のたびに番号順で全ファイルを DB_HOST に適用する。

- 練習環境は TTL で消えるので、DB への変更はここに書かないと次回に再現できない。
- 何度流しても壊れないように書く（PostgreSQL なので CREATE INDEX IF NOT EXISTS などで書く。エラーは deploy を止める）。
- `/initialize` でスキーマを作り直す問題では、初期化処理の側にも入れる必要がある。
