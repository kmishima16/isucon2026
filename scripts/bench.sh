#!/usr/bin/env bash
# make prebench : 計測ログを空にする（本番でポータルからベンチを回す前にも使う）
# make bench    : prebench → BENCH_CMD → スコアを notes/scores.tsv に記録   （PROFILE=1 で pprof も）
# make score S=<点数> [M=メモ] : 手で記録（本番用）
source "$(dirname "$0")/lib.sh"

deployed() { cut -d' ' -f1 "$LOG_DIR/deployed" 2>/dev/null || echo '?'; }
record() {   # pass score memo
  [ -f notes/scores.tsv ] || printf 'time\trev\tpass\tscore\tmemo\n' > notes/scores.tsv
  printf '%s\t%s\t%s\t%s\t%s\n' "$(ts)" "$(deployed)" "$1" "$2" "${3:-$(git log -1 --format=%s "$(deployed)" 2>/dev/null || true)}" >> notes/scores.tsv
  info "記録: rev=$(deployed) pass=$1 score=$2"
}
prebench() {
  rsh "$NGINX_HOST" "sudo truncate -s0 $NGINX_LOG 2>/dev/null || true"
  rsh "$DB_HOST" "sudo truncate -s0 $SLOW_LOG 2>/dev/null || true"
  info "計測ログを空にしました"
  # pprotein はここから PPROTEIN_SECONDS 秒の分を集めるので、ベンチの直前に始める
  if [ -n "${PPROTEIN_AGENT_PORT:-}" ]; then bash "$ROOT/scripts/pprotein.sh" collect; fi
}

case "${1:-bench}" in
  prebench) prebench ;;
  score) record "${PASS:-true}" "${S:?S=<点数> を指定}" "${M:-}" ;;
  bench)
    acquire_lock "bench"
    [ "$(deployed)" = "$(git rev-parse --short HEAD)" ] || warn "HEAD とデプロイ済み($(deployed))が違います。意図どおりですか？"
    if [ -z "${BENCH_CMD:-}" ]; then
      prebench
      info "BENCH_CMD が空です。ポータルからベンチを実行し、終わったら make analyze と make score S=<点数>"
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
    # 結果の形式が違う問題では手で記録する
    if [ -n "$SCORE" ]; then record "${PASS:-?}" "$SCORE"; else warn "スコアを読み取れませんでした。make score S=<点数> で記録してください"; fi
    ;;
esac
