#!/usr/bin/env bash
# scripts/sysstat.sh の集計と、ポータルのベンチで計測の窓を延ばすこと（scripts/bench.sh の prebench）を確かめる
source "$(dirname "$0")/helpers.sh"

echo "集計はベンチ中の秒だけを使う（前後のアイドルで薄めない）"
make_fixture
d="$FIX/m"; mkdir -p "$d"
{
  echo 'procs -----------memory---------- ---swap-- -----io---- -system-- ------cpu----- -----timestamp-----'
  echo ' r  b   swpd   free   buff  cache   si   so    bi    bo   in   cs us sy id wa st                 UTC'
  for _ in 1 2 3; do echo ' 0  0      0 100000   1000 500000    0    0     0     0  100  200  1  0 99  0  0 2026-10-30 10:00:00'; done
  for _ in 1 2 3; do echo ' 4  0      0 100000   1000 500000    0    0     0     0  900  999 60 20 10 10  0 2026-10-30 10:00:10'; done
  for _ in 1 2; do echo ' 0  0      0 100000   1000 500000    0    0     0     0  100  200  1  0 99  0  0 2026-10-30 10:00:20'; done
} > "$d/vmstat.txt"
{
  printf 'Linux 6.8.0 (h) \t10/30/26 \t_x86_64_\t(2 CPU)\n\n'
  printf '# Time        UID       PID    %%usr %%system  %%guest   %%wait    %%CPU   CPU  Command\n\n'
  for _ in 1 2; do
    printf '# Time        UID       PID    %%usr %%system  %%guest   %%wait    %%CPU   CPU  Command\n'
    printf ' 1730282400  1000      1234   40.00   10.00    0.00    0.00   50.00     0  app\n\n'
  done
  printf '# Time        UID       PID    %%usr %%system  %%guest   %%wait    %%CPU   CPU  Command\n\n'
} > "$d/pidstat.txt"
out="$(bash scripts/sysstat.sh summarize "$d" 2>&1 || true)"
contains "vmstat はベンチ中の 3 秒で平均する" "vmstat   3秒  us 60%" "$out"
contains "pidstat は CPU を使っていた区間だけで平均する" "50.0  app" "$out"
finish_fixture

echo "ポータルのベンチ（BENCH_CMD が空）では計測の窓を延ばす"
make_fixture
mkdir -p stub
# ssh の代役：サーバーに送ったコマンドと標準入力を残す
cat > stub/ssh <<'EOF'
#!/usr/bin/env bash
{ echo "${!#}"; [ -t 0 ] || cat; } >> "$SENT"
EOF
chmod +x stub/ssh
printf 'BENCH_CMD=""\nPPROTEIN_AGENT_PORT=""\n' >> isu.env
PATH="$FIX/stub:$PATH" SENT="$FIX/sent" bash scripts/bench.sh prebench >/dev/null 2>&1 || true
contains "ポータルでは PORTAL_MEASURE_SECONDS（既定 240 秒）取る" "timeout 240 vmstat" "$(cat "$FIX/sent")"
: > "$FIX/sent"
printf 'BENCH_CMD="true"\n' >> isu.env
PATH="$FIX/stub:$PATH" SENT="$FIX/sent" bash scripts/bench.sh prebench >/dev/null 2>&1 || true
contains "手元でベンチするときは MEASURE_SECONDS（既定 70 秒）のまま" "timeout 70 vmstat" "$(cat "$FIX/sent")"
finish_fixture

report
