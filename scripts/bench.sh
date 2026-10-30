#!/usr/bin/env bash
# make prebench : 計測ログを空にする（本番でポータルからベンチを回す前にも使う）
# make bench    : prebench → BENCH_CMD → スコアを docs/scores.tsv に記録   （PROFILE=1 で pprof と fgprof も）
#                 E="期待した動き" で、施策の狙い（例: E="GET / の DB 待ちが消える"）を一緒に残す。M=メモ で memo を上書き
# make score S=<点数> [M=メモ] [E=期待] [F=fail の件数] : 手で記録（本番用）
# 記録の fail 列は、make best が fail のある版を選ばないために使う（pass でも fail が出ていることがある）
source "$(dirname "$0")/lib.sh"

record() {   # pass score memo expect fail
  local f=docs/scores.tsv
  [ -f $f ] || printf 'time\trev\tpass\tscore\tmemo\texpect\tfail\n' > $f
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$(ts)" "$(deployed_rev)" "$1" "$2" "${3:-$(git log -1 --format=%s "$(deployed_rev)" 2>/dev/null || true)}" "${4:-}" "${5:-}" >> $f
  info "記録: rev=$(deployed_rev) pass=$1 score=$2 fail=${5:-?}${4:+  期待: $4}"
  # 未コミットのまま残ると次の deploy が止まるので、記録だけをその場でコミットする（ほかの変更は巻き込まない）
  # bench を裏で回すと、操作役の git の操作と重なることがある。index.lock が外れるまで（最長 30 秒）待ってやり直す
  local lock i
  lock="$(git rev-parse --git-dir)/index.lock"
  for i in $(seq 1 30); do
    if git add $f 2>/dev/null && git commit -q -m "chore: ベンチ結果を記録する" -- $f 2>/dev/null; then return 0; fi
    [ -e "$lock" ] || break
    [ "$i" = 1 ] && info "ほかの git の操作が終わるのを待っています（${lock}）"
    sleep 1
  done
  git add $f && git commit -q -m "chore: ベンチ結果を記録する" -- $f || die "$f をコミットできません"
}
prebench() {
  # ポータルのベンチは、いつ始まるか分からない（待ち行列）。窓を長く取り、集計はベンチ中の秒だけを使う（scripts/sysstat.sh）
  if [ -z "${BENCH_CMD:-}" ]; then export MEASURE_SECONDS="$PORTAL_MEASURE_SECONDS" PPROTEIN_SECONDS="$PORTAL_MEASURE_SECONDS"; fi
  rsh "$NGINX_HOST" "sudo truncate -s0 $NGINX_LOG 2>/dev/null || true"
  rsh "$DB_HOST" "sudo truncate -s0 $SLOW_LOG 2>/dev/null || true"
  info "計測ログを空にしました"
  bash "$ROOT/scripts/sysstat.sh" start
  # pprotein はここから PPROTEIN_SECONDS 秒の分を集めるので、ベンチの直前に始める
  if [ -n "${PPROTEIN_AGENT_PORT:-}" ]; then bash "$ROOT/scripts/pprotein.sh" collect; fi
}

refuse_in_worktree "${1:-bench}"
case "${1:-bench}" in
  prebench) prebench ;;
  score) record "${PASS:-true}" "${S:?S=<点数> を指定}" "${M:-}" "${E:-}" "${F:-}" ;;
  bench)
    acquire_lock "bench"
    same_as_deployed || warn "HEAD とデプロイ済み($(deployed_rev))で docs/ 以外が違います。意図どおりですか？"
    if [ -z "${BENCH_CMD:-}" ]; then
      prebench
      info "BENCH_CMD が空です。ポータルからベンチを実行し、make portal WATCH=1 E=\"<期待>\" で記録してから make analyze（取り出せなければ make score S=<点数>）。ポータルのベンチ中は deploy しない（ロックがかからない）"
      exit 0
    fi
    prebench
    T="$(ts)"; echo "$T" > "$LOG_DIR/latest"
    PROF_PID=""
    if [ "${PROFILE:-0}" = 1 ]; then (sleep 5; bash scripts/prof.sh "$T") & PROF_PID=$!; fi
    info "bench: $BENCH_CMD"
    set +e; bash -c "$BENCH_CMD" 2>&1 | tee "$LOG_DIR/$T-bench.txt"; set -e
    if [ -n "$PROF_PID" ]; then wait "$PROF_PID" || true; fi
    RESULT="$(grep -Eo '\{"pass".*\}' "$LOG_DIR/$T-bench.txt" | tail -1 || true)"
    PASS="$(grep -Eo '"pass":(true|false)' <<<"$RESULT" | cut -d: -f2 || true)"
    SCORE="$(grep -Eo '"score":-?[0-9]+' <<<"$RESULT" | cut -d: -f2 || true)"
    FAIL="$(grep -Eo '"fail":[0-9]+' <<<"$RESULT" | cut -d: -f2 || true)"
    # 結果の形式が違う問題では手で記録する
    if [ -n "$SCORE" ]; then record "${PASS:-?}" "$SCORE" "${M:-}" "${E:-}" "$FAIL"; else warn "スコアを読み取れませんでした。make score S=<点数> [F=<fail の件数>] で記録してください"; fi
    ;;
esac
