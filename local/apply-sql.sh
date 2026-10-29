#!/bin/bash
# MySQL コンテナの初期化（docker-entrypoint-initdb.d）で、部分ダンプのあとに sql/*.sql を流す。
# 部分ダンプにはサーバーで足したインデックスが入っているので、deploy と同じく重複（ERROR 1061）だけは適用済みとみなす。
# entrypoint は実行権の見え方しだいで、このファイルを実行も source もするので、どちらでも動く書き方にしている。
for f in /docker-entrypoint-initdb.d/sql/*.sql; do
  [ -e "$f" ] || continue
  echo "apply $(basename "$f")"
  if ! out="$(mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE" --force < "$f" 2>&1)"; then
    if printf '%s\n' "$out" | grep 'ERROR' | grep -qv 'ERROR 1061'; then
      printf '%s\n' "$out"
      exit 1
    fi
  fi
done
