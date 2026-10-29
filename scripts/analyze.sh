#!/usr/bin/env bash
# 直前のベンチ中の計測ログを集計して logs/<時刻>-*.txt に保存する。ツールは自由に足してよい。
source "$(dirname "$0")/lib.sh"
T="$(cat "$LOG_DIR/latest" 2>/dev/null || ts)"
[ -f "$LOG_DIR/$T-bench.txt" ] || { T="$(ts)"; echo "$T" > "$LOG_DIR/latest"; }

info "alp ($NGINX_HOST:$NGINX_LOG) 合計時間の降順"
if rsh "$NGINX_HOST" "sudo test -s $NGINX_LOG"; then
  # alp は既定でアプリ側の時間（apptime）を読む。これまでの JSON と同じく、nginx が受けてから返すまでの時間（reqtime）で集計する
  rsh "$NGINX_HOST" "sudo alp ltsv --apptime-label reqtime --file $NGINX_LOG --sort sum -r -m '$ALP_MATCH' -o count,method,uri,2xx,3xx,4xx,5xx,min,avg,max,sum" > "$LOG_DIR/$T-alp.txt"
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
if [ -s "$LOG_DIR/$T-fgprof-top.txt" ]; then info "fgprof（I/O 待ちを含む）"; head -n 25 "$LOG_DIR/$T-fgprof-top.txt"; fi

# prebench から各台で取っていた vmstat・pidstat・memcached の stats
bash "$ROOT/scripts/sysstat.sh" collect "$T"
info "保存先: logs/$T-*"
