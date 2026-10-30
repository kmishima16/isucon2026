#!/usr/bin/env bash
# pprotein で計測を画面とファイルで見る（Mac 側で動かす）。
#   make pp-up        : pprotein を起動し、各台への ssh トンネルを張り、収集先と alp の設定を入れる
#   make pp-collect   : 収集を始める（PPROTEIN_SECONDS 秒。make prebench からも自動で呼ばれる）
#   make pp-status    : pprotein とトンネルの状態
#   make pp-down      : pprotein とトンネルを止める
#   make pp-agent-off : 各台の pprotein-agent を止める（最終計測の前に）。戻すときは pp-agent-on
# 画面: http://localhost:$PPROTEIN_PORT   ファイル: pprotein/data/（Claude もここを読める）
source "$(dirname "$0")/lib.sh"
[ -n "${PPROTEIN_AGENT_PORT:-}" ] || die "isu.env の PPROTEIN_AGENT_PORT が空です（pprotein を使わない設定）"

PP_DIR="$ROOT/pprotein"; RUN="$PP_DIR/run"; TOOLS="$ROOT/.tools"
URL="http://localhost:$PPROTEIN_PORT"
ALP_LOCAL_VERSION="v1.0.22"
SLP_LOCAL_VERSION="v0.2.3"
# トンネルの手元のポート。台の順番 i（1始まり）に、agent は 19100+i、アプリの pprof は 19200+i
AGENT_BASE=19100; PPROF_BASE=19200
mkdir -p "$RUN" "$TOOLS"

in_list() { [[ " $2 " == *" $1 "* ]]; }
server_up() { curl -fs -o /dev/null --max-time 2 "$URL/api/group/targets"; }
alive() { [ -f "$1" ] && kill -0 "$(cat "$1")" 2>/dev/null; }

# pprotein は alp と slp を外部コマンドとして呼ぶので、3つとも手元に置く
ensure_tools() {
  local os arch v
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; *) arch=amd64 ;; esac
  v="${PPROTEIN_VERSION#v}"
  fetch() {   # 名前 URL
    [ -x "$TOOLS/$1" ] && return 0
    info "ダウンロード: $2"
    curl -fsSL --max-time 120 "$2" | tar xz -C "$TOOLS" "$1"
  }
  fetch pprotein "https://github.com/kaz/pprotein/releases/download/v$v/pprotein_${v}_${os}_$arch.tar.gz"
  fetch alp "https://github.com/tkuchiki/alp/releases/download/$ALP_LOCAL_VERSION/alp_${os}_$arch.tar.gz"
  fetch slp "https://github.com/tkuchiki/slp/releases/download/$SLP_LOCAL_VERSION/slp_${os}_$arch.tar.gz"
}

# 名前 手元のポート ホスト 先のポート
tunnel() {
  local pid="$RUN/tunnel-$1.pid"
  alive "$pid" && return 0
  # shellcheck disable=SC2086
  ssh -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=15 -o ServerAliveCountMax=4 -o BatchMode=yes \
      -o ControlMaster=no -o ControlPath=none ${SSH_EXTRA_OPTS:-} \
      -L "127.0.0.1:$2:127.0.0.1:$4" "$3" >"$RUN/tunnel-$1.log" 2>&1 &
  echo $! > "$pid"
}

# 収集先: nginx の台のアクセスログ、DB の台のスローログ、アプリの台の pprof
targets_json() {
  local i=0 h p items=()
  item() { items+=("$(printf '{"Type":"%s","Label":"%s","URL":"%s","Duration":%d}' "$1" "$2" "$3" "$PPROTEIN_SECONDS")"); }
  for h in $HOSTS; do
    i=$((i + 1))
    [ "$h" = "$NGINX_HOST" ] && item httplog "$h" "http://localhost:$((AGENT_BASE + i))/debug/log/httplog"
    [ "$h" = "$DB_HOST" ] && item slowlog "$h" "http://localhost:$((AGENT_BASE + i))/debug/log/slowlog"
    if [ -n "${PPROTEIN_PPROF_PORT:-}" ] && in_list "$h" "$APP_HOSTS"; then
      for p in $PPROTEIN_PPROF_PATHS; do item pprof "$h-$(basename "$p")" "http://localhost:$((PPROF_BASE + i))$p"; done
    fi
  done
  # macOS の bash 3.2 は set -u で空の配列を展開すると止まる
  if [ "${#items[@]}" -eq 0 ]; then echo "[]"; return; fi
  local IFS=,; echo "[${items[*]}]"
}

# alp の設定: isu.env の ALP_MATCH（カンマ区切りの正規表現）をそのまま使う
alp_yaml() {
  local pats p
  echo "sort: sum"
  echo "reverse: true"
  # make analyze と同じく、nginx が受けてから返すまでの時間（reqtime）で集計する
  echo "ltsv:"
  echo "  apptime_label: reqtime"
  echo "matching_groups:"
  IFS=',' read -ra pats <<<"$ALP_MATCH"
  for p in "${pats[@]}"; do printf "  - '%s'\n" "${p//\'/\'\'}"; done
}

up() {
  ensure_tools
  if ! server_up; then
    info "pprotein を起動（${URL}、データは pprotein/data/）"
    # exec で置き換え、保存する PID を pprotein そのものにする（pp-down で確実に止めるため）
    (cd "$PP_DIR" && PORT="$PPROTEIN_PORT" PATH="$TOOLS:$PATH" exec nohup "$TOOLS/pprotein" >"$RUN/server.log" 2>&1) &
    echo $! >"$RUN/server.pid"
    for _ in $(seq 1 20); do server_up && break; sleep 0.5; done
    server_up || die "pprotein が起動しません（pprotein/run/server.log を確認。ポート $PPROTEIN_PORT が使われていないか）"
  fi

  local i=0 h
  for h in $HOSTS; do
    i=$((i + 1))
    if [ "$h" = "$NGINX_HOST" ] || [ "$h" = "$DB_HOST" ]; then tunnel "agent-$h" $((AGENT_BASE + i)) "$h" "$PPROTEIN_AGENT_PORT"; fi
    if [ -n "${PPROTEIN_PPROF_PORT:-}" ] && in_list "$h" "$APP_HOSTS"; then tunnel "pprof-$h" $((PPROF_BASE + i)) "$h" "$PPROTEIN_PPROF_PORT"; fi
  done
  sleep 2
  local bad=0 f
  for f in "$RUN"/tunnel-*.pid; do
    [ -e "$f" ] || continue
    alive "$f" || { warn "トンネルが張れません: $(basename "$f" .pid)（$(tail -n 3 "${f%.pid}.log")）"; bad=1; }
  done
  [ "$bad" = 0 ] || die "ssh トンネルに失敗しました。make doctor で ssh を確認してください。"

  targets_json | curl -fsS -X POST --data-binary @- "$URL/api/group/targets" || die "収集先を登録できません"
  alp_yaml | curl -fsS -X POST --data-binary @- "$URL/api/httplog/config" || die "alp の設定を登録できません"
  info "準備できました。画面: $URL   収集は make prebench（または make pp-collect）で始まります"
}

collect() {
  if ! server_up; then
    warn "pprotein が起動していないので、収集は省きます（使うなら make pp-up）"
    return 0
  fi
  # 収集の長さは、登録した収集先の Duration で決まる。prebench が窓を延ばしたとき（ポータルのベンチ）にも効くよう、毎回登録し直す
  targets_json | curl -fsS -X POST --data-binary @- "$URL/api/group/targets" >/dev/null || die "収集先を登録し直せません"
  nohup curl -fsS --max-time $((PPROTEIN_SECONDS + 120)) "$URL/api/group/collect" >"$RUN/collect.log" 2>&1 &
  info "pprotein: ${PPROTEIN_SECONDS} 秒の収集を始めました。終わると $URL と pprotein/data/ に出ます"
}

status() {
  if server_up; then info "pprotein: 起動中 $URL"; else warn "pprotein: 止まっています"; fi
  local f
  for f in "$RUN"/tunnel-*.pid; do
    [ -e "$f" ] || continue
    if alive "$f"; then echo "  $(basename "$f" .pid): 張れています"; else echo "  $(basename "$f" .pid): 切れています（make pp-up で張り直す）"; fi
  done
  if server_up; then curl -fsS "$URL/api/group/targets"; echo; fi
}

down() {
  local f
  for f in "$RUN"/*.pid; do
    [ -e "$f" ] || continue
    if alive "$f"; then kill "$(cat "$f")"; fi
    rm -f "$f"
  done
  info "pprotein とトンネルを止めました（データは pprotein/data/ に残っています）"
}

agent() {   # on | off
  local h
  for h in $HOSTS; do
    if [ "$1" = off ]; then rsh "$h" "sudo systemctl disable -q --now pprotein-agent"
    else rsh "$h" "sudo systemctl enable -q --now pprotein-agent"; fi
    echo "  [$h] pprotein-agent: $(rsh "$h" "systemctl is-active pprotein-agent" || true)"
  done
}

case "${1:-status}" in
  up) up ;;
  collect) collect ;;
  status) status ;;
  down) down ;;
  agent-off) refuse_in_worktree pp-agent-off; agent off ;;
  agent-on) refuse_in_worktree pp-agent-on; agent on ;;
  *) die "使い方: pprotein.sh up|collect|status|down|agent-off|agent-on" ;;
esac
