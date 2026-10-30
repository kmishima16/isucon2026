#!/usr/bin/env bash
# scripts/hooks/ の 2 つに見本の入力を与え、出力を確かめる
source "$(dirname "$0")/helpers.sh"
WT=.claude/worktrees/docs
stop_json='{"stop_hook_active":false,"background_tasks":[]}'
ctx() { jq -r '.hookSpecificOutput.additionalContext // ""' <<<"$1"; }

echo "before-stop"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/overview/001_a.md "overview"
git -C "$WT" add docs/overview/001_a.md
git -C "$WT" commit -q -m "docs: overview"
out="$(bash scripts/hooks/before-stop.sh <<<"$stop_json")"
check "Stop の形で出す" "Stop" "$(jq -r .hookSpecificOutput.hookEventName <<<"$out")"
contains "取り込みを促す" "make docs-merge" "$(ctx "$out")"
check "hooks で続けた後は何もしない" "" "$(bash scripts/hooks/before-stop.sh <<<'{"stop_hook_active":true,"background_tasks":[]}')"
check "裏で doc-writer が動いていれば何もしない" "" "$(bash scripts/hooks/before-stop.sh <<<'{"stop_hook_active":false,"background_tasks":[{"type":"subagent","agent_type":"doc-writer","status":"running"}]}')"
out="$(bash scripts/hooks/before-stop.sh <<<'{"stop_hook_active":false,"background_tasks":[{"type":"subagent","agent_type":"Explore","status":"running"}]}')"
contains "ほかのサブエージェントだけなら確かめる" "make docs-merge" "$(ctx "$out")"
check "background_tasks が無い入力でも動く" "Stop" "$(bash scripts/hooks/before-stop.sh <<<'{"stop_hook_active":false}' | jq -r .hookSpecificOutput.hookEventName)"
bash scripts/docs.sh merge >/dev/null 2>&1
check "取り込んだ後は何もしない" "" "$(bash scripts/hooks/before-stop.sh <<<"$stop_json")"
finish_fixture

echo "before-stop（調査役の worktree から）"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/overview/001_a.md "overview"
git -C "$WT" add docs/overview/001_a.md
git -C "$WT" commit -q -m "docs: overview"
git worktree add -q -b research .claude/worktrees/research main
out="$(cd .claude/worktrees/research && bash scripts/hooks/before-stop.sh <<<"$stop_json")"
contains "worktree からでも取り込みを案内する" "make docs-merge" "$(ctx "$out")"
finish_fixture

echo "session-start"
make_fixture
out="$(bash scripts/hooks/session-start.sh <<<'{"source":"startup"}')"
contains "記録が無いとき" "まだ記録がありません" "$out"
contains "役割の確認を出す" "/isu-ops" "$out"
contains "調査役の案内も出す" "/isu-research" "$out"
contains "issue の案内を出す" "gh issue list" "$out"
printf '20261030-110000\tabc\ttrue\t4242\tm\t\n' >> docs/scores.tsv
git add docs && git commit -q -m "chore: ベンチ結果を記録する"
out="$(bash scripts/hooks/session-start.sh <<<'{"source":"clear"}')"
contains "ベストスコアを出す" "4242" "$out"
check "10,000 文字以内" "yes" "$([ "${#out}" -le 10000 ] && echo yes || echo no)"
finish_fixture

report
