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

# 計測の窓はベンチより長い（ポータルのベンチは開始が遅れるので、さらに長く取る）。前後のアイドルで平均が薄まらないよう、ベンチ中の秒だけを集計する
summarize() {   # 回収したディレクトリ
  local d="$1"
  if [ -s "$d/vmstat.txt" ]; then
    # 見出し行から列の位置を拾う（vmstat の版で列が増減するため）。id が 90% 未満の最初と最後の秒の間をベンチ中とみなす（無ければ全部）
    awk '/^ *r +b/{for(i=1;i<=NF;i++)c[$i]=i; next} c["us"] && $1 ~ /^[0-9]+$/ {m++; us[m]=$c["us"]; sy[m]=$c["sy"]; id[m]=$c["id"]; wa[m]=$c["wa"]; st[m]=$c["st"]; r[m]=$c["r"]; if($c["id"]<90){if(!a)a=m; z=m}}
      END{if(!m) exit; if(!a){a=1; z=m}; for(i=a;i<=z;i++){n++; U+=us[i]; S+=sy[i]; I+=id[i]; W+=wa[i]; T+=st[i]; R+=r[i]; if(r[i]>rm)rm=r[i]}
        printf "  vmstat   %d秒  us %.0f%%  sy %.0f%%  id %.0f%%  wa %.0f%%  st %.0f%%  実行待ち r 平均 %.1f / 最大 %d\n", n, U/n, S/n, I/n, W/n, T/n, R/n, rm}' "$d/vmstat.txt"
  fi
  if [ -s "$d/pidstat.txt" ]; then
    local p
    # 区間（見出しから次の見出しまで）の %CPU の合計が 10 未満の区間は、ベンチの外とみなして数えない。1 行目に数えた区間の数を出す
    p="$(awk 'function flush(){ if(tot>=10){busy++; for(k in cur) s[k]+=cur[k]} delete cur; tot=0 }
      /^#/{flush(); for(i=2;i<=NF;i++){if($i=="%CPU")cpu=i-1; if($i=="Command")cmd=i-1}; next}
      cpu && NF>=cmd {cur[$cmd]+=$cpu; tot+=$cpu}
      END{flush(); if(!busy) exit; print busy; for(k in s) printf "%7.1f  %s\n", s[k]/busy, k}' "$d/pidstat.txt")"
    if [ -n "$p" ]; then
      echo "  pidstat  コマンドごとの平均 %CPU（1 コア = 100%、ベンチ中の $(head -1 <<<"$p") 区間）"
      tail -n +2 <<<"$p" | sort -rn | head -n 8 | sed 's/^/    /'
    fi
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
  summarize) summarize "${2:?summarize には回収したディレクトリ（logs/T-<台>）を渡す}" ;;
  *) die "使い方: sysstat.sh start|collect T|summarize DIR" ;;
esac
