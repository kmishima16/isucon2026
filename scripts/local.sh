#!/usr/bin/env bash
# 手元の docker compose でアプリを動かす。巡回（make patrol）は scripts/patrol.sh。
#   make local-seed : サーバーから部分ダンプ（LOCAL_SEED）と静的ファイルを取る（最初の1回。静的ファイルは local/public に入るので commit）
#   make local-up   : 手元で起動（http://localhost:$LOCAL_PORT）。アプリは作業ツリーのコードからビルドする
#   make local-down : 止めて DB を消す（次の local-up で seed と sql/*.sql から作り直す）
# 構成は local/compose.yml。部分ダンプなど取ってきたものは local/data/（git に入れない）。
source "$(dirname "$0")/lib.sh"
L="$ROOT/local"; D="$L/data"
COMPOSE=(docker compose -f "$L/compose.yml")
export LOCAL_PORT

seed() {
  mkdir -p "$D/initdb" "$L/public"
  info "[$DB_HOST] 部分ダンプ: $LOCAL_SEED"
  local spec t w
  for spec in $LOCAL_SEED; do
    t="${spec%%:*}"; w=""; [[ "$spec" == *:* ]] && w="${spec#*:}"
    rsh "$DB_HOST" "sudo mysqldump --no-tablespaces --single-transaction --skip-comments ${w:+--where=$(printf %q "$w")} $LOCAL_DB_NAME $t" \
      || die "$t のダンプに失敗しました"
  done | gzip > "$D/initdb/00_seed.sql.gz"
  info "保存: local/data/initdb/00_seed.sql.gz（$(du -h "$D/initdb/00_seed.sql.gz" | cut -f1)）"
  rsync -rlt --delete --exclude /image -e "$RSYNC_E" "${APP_HOSTS%% *}:$LOCAL_PUBLIC_REMOTE/" "$L/public/"
  info "静的ファイル: local/public/（変わっていれば commit）"
}

up() {
  [ -s "$D/initdb/00_seed.sql.gz" ] || die "部分ダンプがありません。先に make local-seed"
  # sql/*.sql（インデックスなど）を部分ダンプのあとに流す
  rm -rf "$D/initdb/sql" "$D"/initdb/10_*; mkdir -p "$D/initdb/sql"
  local sqls; shopt -s nullglob; sqls=("$ROOT"/sql/*.sql); shopt -u nullglob
  if [ ${#sqls[@]} -gt 0 ]; then cp "${sqls[@]}" "$D/initdb/sql/"; else info "sql/*.sql はありません"; fi
  cp "$L/apply-sql.sh" "$D/initdb/10_apply-sql.sh"
  # サーバーと同じサイト設定を、置き場所とアプリの宛先だけ差し替えて使う
  sed -e "s#${LOCAL_PUBLIC_REMOTE}/\{0,1\}#/public/#g" -e "s#${LOCAL_UPSTREAM}#app:8080#g" "$ROOT/$LOCAL_NGINX_SITE" > "$D/nginx.conf"
  mkdir -p "$D/public"; rsync -rlt "$L/public/" "$D/public/"
  "${COMPOSE[@]}" up -d --build --wait || { "${COMPOSE[@]}" logs --tail 30 app; die "起動できませんでした（上のログ）"; }
  curl -fsS -o /dev/null --max-time 10 "http://localhost:$LOCAL_PORT/" || die "http://localhost:$LOCAL_PORT/ が応答しません"
  info "起動しました: http://localhost:$LOCAL_PORT/（止めるときは make local-down）"
}

case "${1:-}" in
  seed) seed ;;
  up) up ;;
  down) "${COMPOSE[@]}" down -v ;;
  *) die "使い方: local.sh seed|up|down" ;;
esac
