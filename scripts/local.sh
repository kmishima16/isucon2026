#!/usr/bin/env bash
# 手元の docker compose でアプリを動かす。巡回（make patrol）は scripts/patrol.sh。
#   make local-seed : サーバーから部分ダンプ（LOCAL_SEED）と静的ファイルを取る（最初の1回。静的ファイルは local/public に入るので commit）
#   make local-up   : 手元で起動（http://localhost:$LOCAL_PORT）。アプリは作業ツリーのコードからビルドする
#   make local-down : 止めて DB を消す（次の local-up で seed と sql/*.sql から作り直す）
# 構成は local/compose.yml。部分ダンプなど取ってきたものは local/data/（git に入れない）。
source "$(dirname "$0")/lib.sh"
L="$ROOT/local"; D="$L/data"
# 練習と本番のリポジトリで同じ名前を使うと、別の問題の環境（DB のボリュームごと）を作り直してしまうので、リポジトリごとに分ける。
# worktree（検証役）でも main と同じ名前になるよう、git の共通のディレクトリからリポジトリ名を取る
PROJECT="isu-$(basename "$(dirname "$(cd "$(git rev-parse --git-common-dir)" && pwd -P)")" | tr 'A-Z' 'a-z' | sed 's/[^a-z0-9_-]/-/g')"
COMPOSE=(docker compose -p "$PROJECT" -f "$L/compose.yml")
export LOCAL_PORT

seed() {
  # サーバーに ssh して全件をダンプするので、検証役の worktree からは動かさない
  refuse_in_worktree local-seed
  mkdir -p "$D/initdb" "$L/public"
  info "[$DB_HOST] ダンプ: $LOCAL_SEED"
  local spec t w tmp="$D/initdb/.00_seed.sql.gz.tmp"
  rm -f "$tmp"
  # 大きな初期データでも転送を短くするため、サーバーの側で圧縮して送る（gzip の連結はそのまま読める）。
  # 書き終えてから置き換えるのは、検証役が書きかけのダンプを写さないため
  for spec in $LOCAL_SEED; do
    t="${spec%%:*}"; w=""; [[ "$spec" == *:* ]] && w="${spec#*:}"
    rsh "$DB_HOST" "set -o pipefail; sudo mysqldump --no-tablespaces --single-transaction --skip-comments ${w:+--where=$(printf %q "$w")} $LOCAL_DB_NAME $t | gzip" \
      || { rm -f "$tmp"; die "$t のダンプに失敗しました"; }
  done > "$tmp"
  mv "$tmp" "$D/initdb/00_seed.sql.gz"
  info "保存: local/data/initdb/00_seed.sql.gz（$(du -h "$D/initdb/00_seed.sql.gz" | cut -f1)）"
  if [ -z "${LOCAL_PUBLIC_REMOTE:-}" ]; then
    info "LOCAL_PUBLIC_REMOTE が空なので、静的ファイルは取りません"
    return
  fi
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
