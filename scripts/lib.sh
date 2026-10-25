#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# shellcheck source=../isu.env
source "$ROOT/isu.env"

LOG_DIR="$ROOT/logs"; mkdir -p "$LOG_DIR" "$ROOT/notes"

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

# --- deploy / bench を直列化するロック（mkdir 方式で macOS でも動く） ---
LOCK_DIR="$ROOT/.lock"
acquire_lock() {
  if ! mkdir "$LOCK_DIR" 2>/dev/null; then
    die "実行中の操作があります: $(cat "$LOCK_DIR/owner" 2>/dev/null || echo '?')
       終わるのを待つ。残骸なら（そのプロセスが居なければ）rm -rf .lock"
  fi
  echo "$1 (pid $$, $(date '+%H:%M:%S'))" > "$LOCK_DIR/owner"
  trap 'rm -rf "$LOCK_DIR"' EXIT
}

# 実行中のホストにサービスがあるか
has_unit() { rsh "$1" "systemctl list-unit-files --type=service --no-legend '$2.service' 2>/dev/null | grep -q ." ; }
