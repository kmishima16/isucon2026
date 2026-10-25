#!/usr/bin/env bash
# pprof の CPU プロファイルを取る。ベンチ実行中に make prof（本番ではポータルでベンチ開始直後に）。
source "$(dirname "$0")/lib.sh"
[ -n "${PPROF_URL:-}" ] || die "isu.env の PPROF_URL が空です"
T="${1:-$(cat "$LOG_DIR/latest" 2>/dev/null || ts)}"
out="$LOG_DIR/$T-cpu.pprof"
info "pprof 取得中: $PPROF_HOST $PPROF_URL"
rsh "$PPROF_HOST" "curl -fsS --max-time 90 '$PPROF_URL'" > "$out" || { rm -f "$out"; die "pprof を取得できません（アプリに net/http/pprof を入れていますか？）"; }
if command -v go >/dev/null; then
  go tool pprof -top -nodecount=30 "$out" > "$LOG_DIR/$T-pprof.txt" 2>/dev/null && head -n 40 "$LOG_DIR/$T-pprof.txt"
else
  warn "手元に go が無いので集計は省略（$out は保存済み）"
fi
