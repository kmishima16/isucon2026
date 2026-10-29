#!/usr/bin/env bash
# スクリプトの書き間違いを、実行する前に見つける。スクリプトを足したり直したりしたら make lint。
source "$(dirname "$0")/lib.sh"
files=(scripts/*.sh local/*.sh)
ng=0
for f in "${files[@]}"; do bash -n "$f" || ng=1; done
# 変数のすぐ後ろに全角文字（「（」など）があると、C ロケールの bash はその文字まで変数名とみなし、set -u で落ちる。${VAR} と書く
bad="$(LC_ALL=C grep -nE '\$[A-Za-z_][A-Za-z0-9_]*[^ -~[:space:]]' "${files[@]}" | grep -vE '^[^:]+:[0-9]+:[[:space:]]*#' || true)"
if [ -n "$bad" ]; then warn "変数の直後に全角文字があります。\${VAR} と書いてください:"; echo "$bad" | sed 's/^/  /'; ng=1; fi
if command -v shellcheck >/dev/null; then shellcheck -S warning -x "${files[@]}" || ng=1; else info "shellcheck が無いので省略（brew install shellcheck）"; fi
[ $ng = 0 ] && info "問題は見つかりませんでした" || die "直してください"
