#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# shellcheck source=../isu.env
source "$ROOT/isu.env"

# --- 道具の既定値。問題で変わらないものはここに置き、変えたいときだけ isu.env に書く（isu.env が優先） ---
# := は空でも既定値にする。= は isu.env で空にすると「使わない」になるもの
: "${MEASURE_SECONDS:=70}"          # prebench から vmstat・pidstat を取る秒数。ベンチより少し長く
: "${MEMCACHED_PORT:=11211}"        # memcached の stats を取るポート
: "${SNAPSHOT_INIT=/initialize}"    # make snapshot / compare の前に叩いてデータをそろえる（空なら叩かない）
: "${LOCAL_PORT:=8080}"             # make local-up で手元に開くポート
: "${LOCAL_UPSTREAM:=localhost:8080}"  # サイト設定の中のアプリの宛先（手元の nginx では app:8080 に差し替える）
: "${PPROTEIN_VERSION:=v1.2.4}"
: "${PPROTEIN_PORT:=9000}"          # Mac で開く pprotein の画面
: "${PPROTEIN_AGENT_PORT=19000}"    # 各台の pprotein-agent（ssh トンネル経由でだけ届く）。空なら pprotein を使わない
: "${PPROTEIN_SECONDS:=70}"         # pprotein の 1 回の収集の長さ。ベンチより少し長く
: "${PORTAL_JQ_PICK:=.}"            # make portal：結果が配列なら isu.env で '.[0]' などにする
: "${PORTAL_JQ_ID=}"                # 結果を見分ける ID（例: '.id'）。空なら中身で見分ける
: "${PORTAL_JQ_PASS:=.pass}"
: "${PORTAL_JQ_SCORE:=.score}"
: "${PORTAL_JQ_FAIL:=.fail}"      # 無い形のポータルでは空になり、記録の fail は空（make best では ? と出る）
: "${PORTAL_JQ_MESSAGES:=.messages}"

LOG_DIR="$ROOT/logs"; mkdir -p "$LOG_DIR"

info() { printf '\033[36m==> %s\033[0m\n' "$*" >&2; }
warn() { printf '\033[33m[warn] %s\033[0m\n' "$*" >&2; }
die()  { printf '\033[31m[error] %s\033[0m\n' "$*" >&2; exit 1; }
ts()   { date +%Y%m%d-%H%M%S; }

# --- ssh：切れにくく、固まらない設定 ---
#  ConnectTimeout   : 届かないときに延々待たない
#  ServerAlive*     : 回線が死んだら約60秒で切る（ハングしない）
#  ControlMaster    : 接続を使い回して高速化（IP変更後は doctor が張り直す）
# shellcheck disable=SC2206
SSH_OPTS=(-o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=4
          -o ControlMaster=auto -o "ControlPath=/tmp/isu-cm-%C" -o ControlPersist=5m
          -o BatchMode=yes ${SSH_EXTRA_OPTS:-})
rsh()      { ssh "${SSH_OPTS[@]}" "$@"; }
RSYNC_E="ssh ${SSH_OPTS[*]}"
ssh_reset() { ssh "${SSH_OPTS[@]}" -O exit "$1" >/dev/null 2>&1 || true; }

# --- git ---
rev() {
  local r; r="$(git rev-parse --short HEAD 2>/dev/null || echo nogit)"
  [ -n "$(git status --porcelain 2>/dev/null)" ] && r="${r}+dirty"
  echo "$r"
}
# デプロイ済みのコミット（logs/deployed の 1 列目）。まだ deploy していなければ ?
deployed_rev() { cut -d' ' -f1 "$LOG_DIR/deployed" 2>/dev/null || echo '?'; }
# HEAD がデプロイ済みと同じか。docs/ だけの差は同じとみなす（docs の取り込みや記録のコミットでは、サーバーに配るものが変わらないため）
same_as_deployed() {
  local d
  d="$(deployed_rev)"
  [ "$d" != '?' ] && git diff --quiet "$d" HEAD -- . ':(exclude)docs'
}

# --- deploy / bench を直列化するロック（mkdir 方式で macOS でも動く） ---
LOCK_DIR="$ROOT/.lock"
# worktree では .git がファイルになる。git に聞かないので、本体を動かして worktree の参照が壊れていても見分けられる
in_worktree() { [ -f "$ROOT/.git" ]; }
# ロックとデプロイ済みの記録はチェックアウトごとにあるので、worktree から動かすと main の操作とすり抜けて重なる。記録や計測の切り替えのコミットも worktree のブランチに混ざる
refuse_in_worktree() {
  if in_worktree; then die "サーバーを変える操作（$1）は main のチェックアウトで実行してください（ここは worktree: ${ROOT}）"; fi
}
acquire_lock() {
  refuse_in_worktree "$1"
  if ! mkdir "$LOCK_DIR" 2>/dev/null; then
    die "実行中の操作があります: $(cat "$LOCK_DIR/owner" 2>/dev/null || echo '?')
       終わるのを待つ。残骸なら（そのプロセスが居なければ）rm -rf .lock"
  fi
  echo "$1 (pid $$, $(date '+%H:%M:%S'))" > "$LOCK_DIR/owner"
  trap 'rm -rf "$LOCK_DIR"' EXIT
}

# 実行中のホストにサービスがあるか
has_unit() { rsh "$1" "systemctl list-unit-files --type=service --no-legend '$2.service' 2>/dev/null | grep -q ." ; }
