#!/usr/bin/env bash
# 直前のベンチ中の計測ログを集計して logs/<時刻>-*.txt に保存する。ツールは自由に足してよい。
source "$(dirname "$0")/lib.sh"
T="$(cat "$LOG_DIR/latest" 2>/dev/null || ts)"
[ -f "$LOG_DIR/$T-bench.txt" ] || { T="$(ts)"; echo "$T" > "$LOG_DIR/latest"; }

info "alp ($NGINX_HOST:$NGINX_LOG) 合計時間の降順"
if rsh "$NGINX_HOST" "sudo test -s $NGINX_LOG"; then
  rsh "$NGINX_HOST" "sudo alp json --file $NGINX_LOG --sort sum -r -m '$ALP_MATCH' -o count,method,uri,status,min,avg,max,sum" > "$LOG_DIR/$T-alp.txt"
  head -n 30 "$LOG_DIR/$T-alp.txt"
else
  warn "$NGINX_LOG が空です（access_log の設定、またはベンチ前に prebench したか確認）"
fi

info "pt-query-digest ($DB_HOST:$SLOW_LOG)"
if rsh "$DB_HOST" "sudo test -s $SLOW_LOG"; then
  rsh "$DB_HOST" "sudo pt-query-digest --limit 15 $SLOW_LOG" > "$LOG_DIR/$T-slow.txt"
  sed -n '/^# Profile/,/^$/p' "$LOG_DIR/$T-slow.txt"
else
  warn "$SLOW_LOG が空です（slow log が無効か、ログの場所が違う）"
fi

if [ -s "$LOG_DIR/$T-pprof.txt" ]; then info "pprof"; head -n 25 "$LOG_DIR/$T-pprof.txt"; fi
info "保存先: logs/$T-*"
