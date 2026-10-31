#!/bin/bash
# PostgreSQL コンテナの初期化（docker-entrypoint-initdb.d）で、部分ダンプのあとに sql/*.sql を流す。
# sql/*.sql は何度流しても壊れない形で書く決まりなので、deploy と同じくエラーで止める。
# entrypoint は実行権の見え方しだいで、このファイルを実行も source もするので、どちらでも動く書き方にしている。
for f in /docker-entrypoint-initdb.d/sql/*.sql; do
  [ -e "$f" ] || continue
  echo "apply $(basename "$f")"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -f "$f" || exit 1
done
