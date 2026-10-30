#!/usr/bin/env bash
# Claude Code の hooks（PostToolUse、matcher は Bash）。make bench・score・portal の後に、results を書かせる案内を出す。
# 入力：標準入力の JSON（tool_input.command）。出力：hookSpecificOutput.additionalContext（作業は止めない）。
# 成功したかは終了コードではなく記録で判断する（Bash の結果に終了コードが無いため）。手引きは docs/kit/hooks.md。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cmd="$(jq -r '.tool_input.command // ""')"
# direnv exec . make bench なども拾う。make prebench と、文字列の中の make bench は拾わない
re='(^|[[:space:];&|])make[[:space:]]+(bench|score|portal)([[:space:]]|$)'
[[ "$cmd" =~ $re ]] || exit 0
benched="$(bash "$ROOT/scripts/docs.sh" pending | awk -F'\t' '$2 == "benched" { printf "%s ", $1 }')"
[ -n "$benched" ] || exit 0
jq -n --arg m "ベンチの記録が増えました。make analyze の後、doc-writer に results を書かせてください（番号とスラッグは design と同じ）: ${benched% }" \
  '{hookSpecificOutput: {hookEventName: "PostToolUse", additionalContext: $m}}'
