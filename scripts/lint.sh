#!/usr/bin/env bash
# スクリプトの書き間違いと、生きた文書の目次・リンクの漏れを、実行する前に見つける。道具を足したり直したりしたら make lint。
source "$(dirname "$0")/lib.sh"
shopt -s nullglob
files=(scripts/*.sh scripts/hooks/*.sh local/*.sh tests/*.sh)
ng=0
for f in "${files[@]}"; do bash -n "$f" || ng=1; done
# 変数のすぐ後ろに全角文字（「（」など）があると、C ロケールの bash はその文字まで変数名とみなし、set -u で落ちる。${VAR} と書く
bad="$(LC_ALL=C grep -nE '\$[A-Za-z_][A-Za-z0-9_]*[^ -~[:space:]]' "${files[@]}" | grep -vE '^[^:]+:[0-9]+:[[:space:]]*#' || true)"
if [ -n "$bad" ]; then warn "変数の直後に全角文字があります。\${VAR} と書いてください:"; echo "$bad" | sed 's/^/  /'; ng=1; fi
if command -v shellcheck >/dev/null; then shellcheck -S warning -x "${files[@]}" || ng=1; else info "shellcheck が無いので省略（brew install shellcheck）"; fi

# README は目次なので、docs/kit と docs/flow のすべての文書にリンクする。生きた文書のリンク先は実在する
toc="$(
  for f in docs/kit/*.md docs/flow/*.md; do
    grep -qF "]($f)" README.md || echo "README.md に $f へのリンクがありません"
  done
  for f in README.md CLAUDE.md docs/README.md docs/kit/*.md docs/flow/*.md; do
    [ -e "$f" ] || continue
    awk '{ s = $0; while (match(s, /\]\([^)]+\)/)) { print substr(s, RSTART + 2, RLENGTH - 3); s = substr(s, RSTART + RLENGTH) } }' "$f" |
      while IFS= read -r link; do
        link="${link%%#*}"
        case "$link" in '' | http://* | https://* | mailto:*) continue ;; esac
        [ -e "$(dirname "$f")/$link" ] || echo "$f のリンク先がありません: $link"
      done
  done
)"
if [ -n "$toc" ]; then warn "目次とリンク:"; echo "$toc" | sed 's/^/  /'; ng=1; fi
[ $ng = 0 ] && info "問題は見つかりませんでした" || die "直してください"
