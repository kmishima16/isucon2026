#!/usr/bin/env bash
# Playwright で主な画面と操作を巡回し、ベンチでは気づきにくい表示崩れや 5xx を拾う。
#   make patrol                : 手元の環境（make local-up）を巡回
#   make patrol TARGET=server  : アプリの台に ssh トンネルを張って、本物を巡回
# 巡回の中身は patrol/tests/ にある（問題ごとに書き直す）。結果とスクリーンショットは patrol/test-results/。
source "$(dirname "$0")/lib.sh"
TUNNEL_PORT=18080   # TARGET=server のとき、アプリの台の 80 番を手元のこのポートに出す

case "${TARGET:-local}" in
  local) base="http://localhost:${LOCAL_PORT:-8080}" ;;
  server)
    # 80 番をそのまま外から叩かず、ssh の中を通す。
    # 出力を閉じておかないと、残ったトンネルがパイプを握って make patrol | ... が終わらない
    ssh -N -o ExitOnForwardFailure=yes -o BatchMode=yes -o ControlMaster=no -o ControlPath=none ${SSH_EXTRA_OPTS:-} \
      -L "127.0.0.1:$TUNNEL_PORT:127.0.0.1:80" "${APP_HOSTS%% *}" </dev/null >/dev/null 2>&1 &
    TUNNEL_PID=$!
    trap 'kill "$TUNNEL_PID" 2>/dev/null' EXIT
    sleep 2; base="http://localhost:$TUNNEL_PORT" ;;
  *) die "TARGET は local か server" ;;
esac

cd "$ROOT/patrol"
[ -d node_modules ] || npm ci --no-audit --no-fund
npx playwright install chromium >/dev/null
info "巡回: ${base}（結果とスクリーンショットは patrol/test-results/）"
BASE_URL="$base" npx playwright test
