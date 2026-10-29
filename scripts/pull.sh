#!/usr/bin/env bash
# サーバーのものをリポジトリに取り込む。
#   make pull-app                   アプリ一式（APP_REMOTE → APP_LOCAL）
#   make pull F=/etc/nginx/nginx.conf         全台共通の設定として server/all/ に
#   make pull F=/etc/mysql H=private-isu-2    そのホスト専用として server/<H>/ に（ディレクトリも可）
source "$(dirname "$0")/lib.sh"

if [ "${1:-}" = app ]; then
  h="${APP_HOSTS%% *}"
  if [ -n "$(ls -A "$APP_LOCAL" 2>/dev/null)" ] && [ "${FORCE:-0}" != 1 ]; then
    die "$APP_LOCAL は既にあります。上書きするなら FORCE=1"
  fi
  EXCLUDES=(); for e in $APP_EXCLUDE; do EXCLUDES+=(--exclude "$e"); done
  mkdir -p "$APP_LOCAL"
  rsync -rlptz "${EXCLUDES[@]}" -e "$RSYNC_E" --rsync-path="sudo rsync" "$h:$APP_REMOTE/" "$APP_LOCAL/"
  info "取り込みました: ${APP_LOCAL}（commit してください）"
  exit 0
fi

F="${F:?F=/絶対パス を指定してください}"
case "$F" in /*) ;; *) die "F は絶対パスで指定してください" ;; esac
H="${H:-}"; h="${H:-${HOSTS%% *}}"; dest="server/${H:-all}$F"
mkdir -p "$(dirname "$dest")"
if rsh "$h" "sudo test -d '$F'"; then
  rsync -rlpt -e "$RSYNC_E" --rsync-path="sudo rsync" "$h:$F/" "$dest/"
else
  rsync -lpt -e "$RSYNC_E" --rsync-path="sudo rsync" "$h:$F" "$dest"
fi
info "取り込みました: ${dest}（編集して commit → make deploy）"
