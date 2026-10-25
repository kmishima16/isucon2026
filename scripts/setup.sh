#!/usr/bin/env bash
# 新しいサーバーに1回だけ実行する（何度実行しても同じ結果になる）。
# 計測ツールを入れて、isu.env の SETUP_CMD を実行する。設定ファイルの反映は deploy の役目。
source "$(dirname "$0")/lib.sh"
ALP_VERSION="${ALP_VERSION:-v1.0.21}"

for h in $HOSTS; do
  info "[$h] setup"
  rsh "$h" true || die "[$h] に ssh できません。make doctor を実行してください。"
  rsh "$h" "sudo ALP_VERSION=$ALP_VERSION bash -s" <<'REMOTE'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
need=()
command -v pt-query-digest >/dev/null || need+=(percona-toolkit)
command -v htop >/dev/null || need+=(htop)
command -v dstat >/dev/null || need+=(dstat)
if [ ${#need[@]} -gt 0 ]; then
  apt-get update -qq && apt-get install -y -qq "${need[@]}" >/dev/null || echo "  [warn] apt で入らなかったもの: ${need[*]}"
fi
if ! command -v alp >/dev/null; then
  curl -fsSL --max-time 60 "https://github.com/tkuchiki/alp/releases/download/${ALP_VERSION}/alp_linux_$(dpkg --print-architecture).tar.gz" \
    | tar xz -C /usr/local/bin alp || echo "  [warn] alp の導入に失敗"
fi
for t in alp pt-query-digest htop dstat; do printf '  %-16s %s\n' "$t" "$(command -v $t || echo '-')"; done
REMOTE
  if [ -n "${SETUP_CMD:-}" ]; then
    info "[$h] SETUP_CMD"
    rsh "$h" "sudo bash -c $(printf %q "$SETUP_CMD")"
  fi
done
info "setup 完了。次は make deploy（初回は make pull-app と commit が先）"
