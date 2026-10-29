#!/usr/bin/env bash
# 手元の docker compose でアプリを動かし、Playwright で主な画面と操作を巡回する。
#   make local-seed : サーバーから部分ダンプ（LOCAL_SEED）と静的ファイルを取る（最初の1回。静的ファイルは local/public に入るので commit）
#   make local-up   : 手元で起動（http://localhost:$LOCAL_PORT）。アプリは作業ツリーのコードからビルドする
#   make local-down : 止めて DB を消す（次の local-up で seed と sql/*.sql から作り直す）
#   make patrol [TARGET=local|server] : Playwright で巡回。server はアプリの台に ssh トンネルを張って、本物を巡回する
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
  cp "$ROOT"/sql/*.sql "$D/initdb/sql/" 2>/dev/null || info "sql/*.sql はありません"
  cp "$L/apply-sql.sh" "$D/initdb/10_apply-sql.sh"
  # サーバーと同じサイト設定を、置き場所とアプリの宛先だけ差し替えて使う
  sed -e "s#${LOCAL_PUBLIC_REMOTE}/\{0,1\}#/public/#g" -e "s#${LOCAL_UPSTREAM}#app:8080#g" "$ROOT/$LOCAL_NGINX_SITE" > "$D/nginx.conf"
  mkdir -p "$D/public"; rsync -rlt "$L/public/" "$D/public/"
  "${COMPOSE[@]}" up -d --build --wait || { "${COMPOSE[@]}" logs --tail 30 app; die "起動できませんでした（上のログ）"; }
  curl -fsS -o /dev/null --max-time 10 "http://localhost:$LOCAL_PORT/" || die "http://localhost:$LOCAL_PORT/ が応答しません"
  info "起動しました: http://localhost:$LOCAL_PORT/（止めるときは make local-down）"
}

patrol() {
  local base
  case "${TARGET:-local}" in
    local) base="http://localhost:$LOCAL_PORT" ;;
    server)
      # 80 番をそのまま外から叩かず、ssh の中を通して手元の 18080 に出す
      # 出力を閉じておかないと、残ったトンネルがパイプを握って make patrol | ... が終わらない。
      # pid は EXIT の trap から見えるよう、local にしない
      ssh -N -o ExitOnForwardFailure=yes -o BatchMode=yes -o ControlMaster=no -o ControlPath=none ${SSH_EXTRA_OPTS:-} \
        -L 127.0.0.1:18080:127.0.0.1:80 "${APP_HOSTS%% *}" </dev/null >/dev/null 2>&1 &
      TUNNEL_PID=$!
      trap 'kill "$TUNNEL_PID" 2>/dev/null' EXIT
      sleep 2; base="http://localhost:18080" ;;
    *) die "TARGET は local か server" ;;
  esac
  cd "$L/e2e"
  [ -d node_modules ] || npm ci --no-audit --no-fund
  npx playwright install chromium >/dev/null
  info "巡回: ${base}（結果とスクリーンショットは local/e2e/test-results/）"
  BASE_URL="$base" npx playwright test
}

case "${1:-}" in
  seed) seed ;;
  up) up ;;
  down) "${COMPOSE[@]}" down -v ;;
  patrol) patrol ;;
  *) die "使い方: local.sh seed|up|down|patrol" ;;
esac
