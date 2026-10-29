#!/usr/bin/env bash
# pprof の CPU プロファイルと fgprof（I/O 待ちも含めた時間）を同時に取る。
# ベンチ実行中に make prof（本番ではポータルでベンチ開始直後に）。make bench PROFILE=1 でも呼ばれる。
source "$(dirname "$0")/lib.sh"
[ -n "${PPROF_URL:-}" ] || die "isu.env の PPROF_URL が空です"
T="${1:-$(cat "$LOG_DIR/latest" 2>/dev/null || ts)}"

fetch() {   # 名前 URL
  local out="$LOG_DIR/$T-$1.pprof"
  if rsh "$PPROF_HOST" "curl -fsS --max-time 90 '$2'" > "$out"; then return 0; fi
  rm -f "$out"; warn "$1 を取得できません: $2（アプリに入れていますか？ make measure-show で確認）"; return 1
}

info "pprof 取得中: $PPROF_HOST $PPROF_URL${FGPROF_URL:+ と $FGPROF_URL}"
pids=()
fetch cpu "$PPROF_URL" & pids+=($!)
if [ -n "${FGPROF_URL:-}" ]; then fetch fgprof "$FGPROF_URL" & pids+=($!); fi
failed=0; for p in "${pids[@]}"; do wait "$p" || failed=1; done

# 手元に go が無ければ、アプリをビルドしているサーバーの go で集計する
top() {   # pprof ファイル
  if go version >/dev/null 2>&1; then go tool pprof -top -nodecount=30 "$1" 2>/dev/null; return; fi
  rsh "$PPROF_HOST" "f=\$(mktemp); cat > \$f; chmod 644 \$f; sudo -u $APP_USER -i bash -c 'export PATH=\$PATH:/usr/local/go/bin:\$HOME/.local/go/bin; go tool pprof -top -nodecount=30 '\$f 2>/dev/null; rm -f \$f" < "$1"
}
for k in cpu fgprof; do
  [ -s "$LOG_DIR/$T-$k.pprof" ] || continue
  top "$LOG_DIR/$T-$k.pprof" > "$LOG_DIR/$T-$k-top.txt" || warn "go tool pprof で $k を集計できません"
done
if [ -s "$LOG_DIR/$T-cpu-top.txt" ]; then cp "$LOG_DIR/$T-cpu-top.txt" "$LOG_DIR/$T-pprof.txt"; head -n 40 "$LOG_DIR/$T-pprof.txt"; fi
[ $failed = 0 ] || die "取得できなかったプロファイルがあります（上の警告）"
