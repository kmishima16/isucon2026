#!/usr/bin/env bash
# 新しいサーバーに1回だけ実行する（何度実行しても同じ結果になる）。
# 計測ツールを入れて、isu.env の SETUP_CMD を実行する。設定ファイルの反映は deploy の役目。
source "$(dirname "$0")/lib.sh"
refuse_in_worktree setup
ALP_VERSION="${ALP_VERSION:-v1.0.21}"

for h in $HOSTS; do
  info "[$h] setup"
  rsh "$h" true || die "[$h] に ssh できません。make doctor を実行してください。"
  rsh "$h" "sudo ALP_VERSION=$ALP_VERSION bash -s" <<'REMOTE'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
# ベンチ中に apt の自動更新や snap の更新が走ると CPU とディスクを取られ、スコアがぶれる。
# 実行中の apt は途中で止めず（dpkg が壊れる）、タイマーとサービスだけ止めてロックが空くのを待つ
for u in apt-daily.timer apt-daily-upgrade.timer unattended-upgrades.service snapd.service snapd.socket snapd.seeded.service; do
  systemctl list-unit-files --no-legend "$u" | grep -q . || continue
  systemctl disable --now -q "$u"
  printf '  %-28s %s\n' "$u" "$(systemctl is-active "$u")"
done
for _ in $(seq 1 60); do
  fuser /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock /var/lib/apt/lists/lock >/dev/null 2>&1 || break
  echo "  apt のロック待ち（自動更新が実行中）"; sleep 5
done
APT=(apt-get -o DPkg::Lock::Timeout=300 -qq)
need=()
command -v pt-query-digest >/dev/null || need+=(percona-toolkit)
command -v htop >/dev/null || need+=(htop)
command -v dstat >/dev/null || need+=(dstat)
command -v pidstat >/dev/null || need+=(sysstat)
if [ ${#need[@]} -gt 0 ]; then
  "${APT[@]}" update && "${APT[@]}" install -y "${need[@]}" >/dev/null || echo "  [warn] apt で入らなかったもの: ${need[*]}"
fi
if ! command -v alp >/dev/null; then
  curl -fsSL --max-time 60 "https://github.com/tkuchiki/alp/releases/download/${ALP_VERSION}/alp_linux_$(dpkg --print-architecture).tar.gz" \
    | tar xz -C /usr/local/bin alp || echo "  [warn] alp の導入に失敗"
fi
for t in alp pt-query-digest htop dstat pidstat; do printf '  %-16s %s\n' "$t" "$(command -v $t || echo '-')"; done
REMOTE
  if [ -n "${PPROTEIN_AGENT_PORT:-}" ]; then
    info "[$h] pprotein-agent"
    rsh "$h" "sudo PP_VERSION=${PPROTEIN_VERSION#v} PP_PORT=$PPROTEIN_AGENT_PORT PP_HTTPLOG=$NGINX_LOG PP_SLOWLOG=$SLOW_LOG bash -s" <<'REMOTE'
set -euo pipefail
# 入っている版と違うときだけ入れ直す
if [ "$(cat /usr/local/share/pprotein-agent.version 2>/dev/null)" != "$PP_VERSION" ]; then
  tmp="$(mktemp -d)"
  curl -fsSL --max-time 120 "https://github.com/kaz/pprotein/releases/download/v${PP_VERSION}/pprotein_${PP_VERSION}_linux_$(dpkg --print-architecture).tar.gz" \
    | tar xz -C "$tmp" pprotein-agent
  install -m 755 "$tmp/pprotein-agent" /usr/local/bin/pprotein-agent
  mkdir -p /usr/local/share && echo "$PP_VERSION" > /usr/local/share/pprotein-agent.version
  rm -rf "$tmp"
fi
cat > /etc/systemd/system/pprotein-agent.service <<UNIT
[Unit]
Description=pprotein-agent (isucon-kit)
After=network.target

[Service]
Environment=PORT=$PP_PORT
Environment=PPROTEIN_HTTPLOG=$PP_HTTPLOG
Environment=PPROTEIN_SLOWLOG=$PP_SLOWLOG
ExecStart=/usr/local/bin/pprotein-agent
Restart=always
# 認証が無いので、この台の localhost（ssh トンネルの出口）からの接続だけを受ける。
# ファイアウォールには触らず、このサービスだけに効く systemd の機能で絞る
IPAddressAllow=localhost
IPAddressDeny=any

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable -q pprotein-agent
systemctl restart pprotein-agent
printf '  %-16s %s (port %s)\n' pprotein-agent "$(systemctl is-active pprotein-agent)" "$PP_PORT"
REMOTE
  fi
  if [ -n "${SETUP_CMD:-}" ]; then
    info "[$h] SETUP_CMD"
    rsh "$h" "sudo bash -c $(printf %q "$SETUP_CMD")"
  fi
done
info "setup 完了。次は make deploy（初回は make pull-app と commit が先）"
