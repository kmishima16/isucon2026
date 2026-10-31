#!/usr/bin/env bash
# PostgreSQL のログを slp で集計する。pprotein（v1.2.4）は slp を「slp my ...」で呼ぶと決め打ちしているので、
# SLOW_LOG_KIND=pg のとき pprotein.sh がこのスクリプトを .tools/slp として挟む。make analyze からも使う。
#   使い方: slp-pg.sh <本物の slp> <log_line_prefix> my|pg <slp の引数...>
# slp pg は「statement:」の行しか読まない。アプリ（pgx）は拡張問い合わせなので、ログは parse・bind・execute の 3 行に分かれる。
# execute の行を statement: の形に直し、parse と bind の行（続きの行も）を捨てる。bind に入る計画の時間は数えない
set -euo pipefail
real="$1"; prefix="$2"; shift 2
case "${1:-}" in
  my|pg) shift ;;
  *) exec "$real" "$@" ;;
esac

args=(); file=""
while [ $# -gt 0 ]; do
  if [ "$1" = --file ]; then file="$2"; shift 2; continue; fi
  args+=("$1"); shift
done
[ -n "$file" ] || { echo "slp-pg.sh: --file がありません" >&2; exit 2; }

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
# 行頭が日時（%m）の行が 1 件の始まり。それ以外は前の行の続き（複数行の SQL）
awk '
  /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] / {
    skip = ($0 ~ /duration: [0-9.]+ ms  (parse|bind) [^:]*: /)
    if (!skip) sub(/ ms  execute [^:]*: /, " ms  statement: ")
  }
  !skip { print }
' "$file" >"$tmp"
"$real" pg --log-line-prefix "$prefix" "${args[@]}" --file "$tmp"
