#!/usr/bin/env bash
# ベンチの間の各台のリソース（vmstat・pidstat・memcached の stats）を取って集計する。
#   sysstat.sh start      : 各台で MEASURE_SECONDS 秒の vmstat・pidstat を始め、memcached の stats を取る（make prebench から）
#   sysstat.sh collect T  : 各台から回収して logs/T-<台>/ に置き、summary.txt に集計する（make analyze から）
# 台の上で取るので、ポータルからベンチを回す本番でも prebench → ベンチ → analyze の同じ手順で使える。
source "$(dirname "$0")/lib.sh"

REMOTE_DIR=/tmp/isu-measure
# memcached が動いている台なら stats を出す（動いていなければ何も出さない）
MEMCACHED_STATS="if systemctl is-active -q memcached; then (exec 3<>/dev/tcp/127.0.0.1/$MEMCACHED_PORT; printf 'stats\r\nquit\r\n' >&3; cat <&3); fi"

start() {
  local h
  for h in $HOSTS; do
    rsh "$h" "sudo bash -s" <<EOF
set -e
rm -rf $REMOTE_DIR; mkdir -p $REMOTE_DIR
nohup timeout $MEASURE_SECONDS vmstat -n -t 1 > $REMOTE_DIR/vmstat.txt 2>&1 < /dev/null &
if command -v pidstat >/dev/null; then LC_ALL=C nohup timeout $MEASURE_SECONDS pidstat -u -h 1 > $REMOTE_DIR/pidstat.txt 2>&1 < /dev/null & fi
$MEMCACHED_STATS > $REMOTE_DIR/memcached-before.txt
EOF
  done
  info "vmstat・pidstat・memcached の stats を $MEASURE_SECONDS 秒取ります"
}

summarize() {   # 回収したディレクトリ
  local d="$1"
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
}

collect() {   # T
  local T="$1" h d
  for h in $HOSTS; do
    d="$LOG_DIR/$T-$h"
    rsh "$h" "test -d $REMOTE_DIR" 2>/dev/null || { warn "[$h] $REMOTE_DIR が無い（prebench 後に再起動した？）"; continue; }
    mkdir -p "$d"
    rsh "$h" "sudo bash -s" > "$d/memcached-after.txt" <<<"$MEMCACHED_STATS"
    rsh "$h" "sudo tar -C $REMOTE_DIR -cf - ." | tar -x -C "$d"
    summarize "$d" > "$d/summary.txt"
    if [ -s "$d/summary.txt" ]; then info "[$h] リソース"; cat "$d/summary.txt"; fi
  done
}

case "${1:-}" in
  start) start ;;
  collect) collect "${2:?collect には T（logs のファイル名の時刻）を渡す}" ;;
  *) die "使い方: sysstat.sh start|collect T" ;;
esac
