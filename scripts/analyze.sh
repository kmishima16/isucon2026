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

# --- prebench から各台で取っていた vmstat・pidstat・memcached の stats ---
for h in $HOSTS; do
  d="$LOG_DIR/$T-$h"
  rsh "$h" "test -d /tmp/isu-measure" 2>/dev/null || { warn "[$h] /tmp/isu-measure が無い（prebench 後に再起動した？）"; continue; }
  mkdir -p "$d"
  rsh "$h" "sudo bash -s" > "$d/memcached-after.txt" <<'EOF'
if systemctl is-active -q memcached; then (exec 3<>/dev/tcp/127.0.0.1/11211; printf 'stats\r\nquit\r\n' >&3; cat <&3); fi
EOF
  rsh "$h" "sudo tar -C /tmp/isu-measure -cf - ." | tar -x -C "$d"
  {
    if [ -s "$d/vmstat.txt" ]; then
      # 見出し行から列の位置を拾う（vmstat の版で列が増減するため）
      awk '/^ *r +b/{for(i=1;i<=NF;i++)c[$i]=i; next} c["us"] && $1 ~ /^[0-9]+$/ {n++; us+=$c["us"]; sy+=$c["sy"]; id+=$c["id"]; wa+=$c["wa"]; st+=$c["st"]; r+=$c["r"]; if($c["r"]>rm)rm=$c["r"]}
        END{if(n) printf "  vmstat   %d秒  us %.0f%%  sy %.0f%%  id %.0f%%  wa %.0f%%  st %.0f%%  実行待ち r 平均 %.1f / 最大 %d\n", n, us/n, sy/n, id/n, wa/n, st/n, r/n, rm}' "$d/vmstat.txt"
    fi
    if [ -s "$d/pidstat.txt" ]; then
      echo "  pidstat  コマンドごとの平均 %CPU（1 コア = 100%）"
      awk '/^#/{for(i=2;i<=NF;i++){if($i=="%CPU")cpu=i-1; if($i=="Command")cmd=i-1}; n++; next} cpu && NF>=cmd {s[$cmd]+=$cpu}
        END{for(k in s) printf "    %7.1f  %s\n", s[k]/n, k}' "$d/pidstat.txt" | sort -rn | head -n 8
    fi
    if [ -s "$d/memcached-before.txt" ] && [ -s "$d/memcached-after.txt" ]; then
      awk 'FNR==1{f++} /^STAT/{gsub(/\r/,""); v[f,$2]=$3}
        END{h=v[2,"get_hits"]-v[1,"get_hits"]; m=v[2,"get_misses"]-v[1,"get_misses"];
            printf "  memcached  get %d（hit %d / miss %d, hit率 %.1f%%）  set %d  evictions +%d  items %d  使用 %.1fMB / %.0fMB\n",
              h+m, h, m, (h+m ? 100*h/(h+m) : 0), v[2,"cmd_set"]-v[1,"cmd_set"], v[2,"evictions"]-v[1,"evictions"],
              v[2,"curr_items"], v[2,"bytes"]/1048576, v[2,"limit_maxbytes"]/1048576}' "$d/memcached-before.txt" "$d/memcached-after.txt"
    fi
  } > "$d/summary.txt"
  if [ -s "$d/summary.txt" ]; then info "[$h] リソース"; cat "$d/summary.txt"; fi
done
info "保存先: logs/$T-*"
