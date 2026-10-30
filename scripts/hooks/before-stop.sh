#!/usr/bin/env bash
# Claude Code の hooks（Stop）。作業を終える前に、取りこぼし（main に入っていない docs、ベンチ済みで results の無い design）を確かめ、
# あれば 1 回だけ作業を続けさせる。入力：標準入力の JSON（stop_hook_active・background_tasks）。手引きは docs/kit/hooks.md。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
input="$(cat)"
# hooks で続けさせた後なら何もしない（無限に続けさせないため）
if [ "$(jq -r '.stop_hook_active // false' <<<"$input")" = true ]; then exit 0; fi
# 裏で doc-writer が動いていれば、終わったときの知らせで Claude がまた動くので、ここでは何もしない
writers="$(jq '[(.background_tasks // [])[] | select(.type == "subagent" and .agent_type == "doc-writer")] | length' <<<"$input")"
[ "$writers" = 0 ] || exit 0
n="$(bash "$ROOT/scripts/docs.sh" unmerged)"
benched="$(bash "$ROOT/scripts/docs.sh" pending | awk -F'\t' '$2 == "benched" { printf "%s ", $1 }')"
todo=""
[ "$n" = 0 ] || todo="main に入っていない docs のコミットが ${n} 件あります。make docs-merge で取り込んでください。"
[ -z "$benched" ] || todo="${todo}ベンチ済みで results の無い design があります（${benched% }）。make analyze の後、doc-writer に results を書かせ、終わったら make docs-merge してください。"
[ -n "$todo" ] || exit 0
jq -n --arg m "$todo" '{hookSpecificOutput: {hookEventName: "Stop", additionalContext: $m}}'
