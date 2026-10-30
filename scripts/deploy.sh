#!/usr/bin/env bash
# コミットをサーバーへ反映する。  make deploy [REV=<commit>]
#   - 反映するのは常に「コミット済みの状態」。未コミットの変更があれば止まる。
#   - REV で過去のコミットをそのまま反映できる（ロールバック）。
source "$(dirname "$0")/lib.sh"
acquire_lock "deploy"

REV="${REV:-HEAD}"
if [ "$REV" = HEAD ] && [ -n "$(git status --porcelain)" ]; then
  git status --short >&2
  die "未コミットの変更があります。commit してから deploy してください（デプロイ＝コミットにするため）。"
fi
SHA="$(git rev-parse --short "$REV")" || die "REV=$REV が見つかりません"

# --- ロックアウト防止：ネットワーク・ssh・権限まわりのファイルは配らない ---
FORBIDDEN='^server/[^/]+/(etc/ssh/|etc/netplan/|etc/network/|etc/systemd/network/|etc/resolv\.conf|etc/hosts$|etc/ufw/|etc/iptables|etc/nftables|etc/sudoers|etc/cloud/|root/\.ssh/|home/[^/]+/\.ssh/)'
bad="$(git ls-tree -r --name-only "$SHA" -- server | grep -E "$FORBIDDEN" || true)"
[ -z "$bad" ] || die "ネットワーク・ssh・権限に関わるファイルは配れません（ロックアウト防止）:
$bad"

# --- コミットの中身を一時ディレクトリに展開（作業ツリーではなくコミットを配る） ---
WORK="$(mktemp -d)"; trap 'rm -rf "$WORK" "$LOCK_DIR"' EXIT
git archive "$SHA" | tar -x -C "$WORK"
T="$(ts)"
EXCLUDES=(); for e in $APP_EXCLUDE; do EXCLUDES+=(--exclude "$e"); done

for h in $HOSTS; do
  info "[$h] 設定ファイル"
  STAGE="/tmp/isu-stage-$T"
  rsh "$h" "rm -rf $STAGE && mkdir -p $STAGE"
  for d in all "$h"; do
    if [ -d "$WORK/server/$d" ]; then rsync -rlpt -e "$RSYNC_E" "$WORK/server/$d/" "$h:$STAGE/"; fi
  done
  rsh "$h" "sudo bash -s -- $STAGE /var/backups/isu-deploy/$T-$SHA" < scripts/apply-overlay.sh \
    || die "[$h] 設定の反映に失敗。上のメッセージを確認（nginx・MySQL の設定が原因なら、元に戻してあります）。"
done

for h in $APP_HOSTS; do
  info "[$h] アプリを転送してビルド"
  rsync -rlptz --delete "${EXCLUDES[@]}" -e "$RSYNC_E" --rsync-path="sudo -u $APP_USER rsync" \
    "$WORK/$APP_LOCAL/" "$h:$APP_REMOTE/"
  rsh "$h" "sudo -u $APP_USER -i bash -s" <<EOF || die "[$h] ビルド失敗（サービスは古いバイナリのまま動いています）"
set -e
export PATH="\$PATH:/usr/local/go/bin:\$HOME/.local/go/bin:\$HOME/go/bin"
cd "$APP_REMOTE"
$BUILD_CMD
EOF
done

shopt -s nullglob
sqls=("$WORK"/sql/*.sql)
if [ ${#sqls[@]} -gt 0 ]; then
  info "[$DB_HOST] sql/*.sql を適用（インデックス重複 ERROR 1061 は適用済みとみなす）"
  for f in "${sqls[@]}"; do
    errs="$(rsh "$DB_HOST" "$MYSQL_CMD --force 2>&1 >/dev/null" < "$f" | grep -v 'password on the command line' | grep -v 'ERROR 1061' || true)"
    [ -z "$errs" ] || die "$(basename "$f") でエラー:
$errs"
  done
fi

for h in $APP_HOSTS; do
  info "[$h] $SERVICE を再起動して疎通確認"
  rsh "$h" "sudo systemctl restart $SERVICE"
  ok=0
  for _ in $(seq 1 30); do
    if rsh "$h" "curl -fsS -o /dev/null --max-time 3 '$HEALTH_URL'"; then ok=1; break; fi
    sleep 1
  done
  if [ "$ok" = 0 ]; then
    rsh "$h" "sudo journalctl -u $SERVICE -n 30 --no-pager" >&2 || true
    die "[$h] $HEALTH_URL が応答しません。直すか、make best で良かったコミットを探して make deploy REV=<rev>"
  fi
done

echo "$SHA $(date '+%F %T')" > "$LOG_DIR/deployed"
info "deploy 完了: $SHA $(git log -1 --format=%s "$SHA")"
