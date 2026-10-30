#!/usr/bin/env bash
# scripts/hooks/ の 3 つに見本の入力を与え、出力を確かめる
source "$(dirname "$0")/helpers.sh"
WT=.claude/worktrees/docs
bench_json='{"tool_input":{"command":"direnv exec . make bench E=x"},"tool_response":{"stdout":"","stderr":"","interrupted":false}}'
stop_json='{"stop_hook_active":false,"background_tasks":[]}'
ctx() { jq -r '.hookSpecificOutput.additionalContext // ""' <<<"$1"; }

echo "after-bash"
make_fixture
write_doc . docs/design/001_a.md "design"
git add docs/design/001_a.md
commit_at . "2020-01-01 00:00:00" "docs: design"
printf '20261030-110000\tabc\ttrue\t100\tm\t\n' >> docs/scores.tsv
git add docs/scores.tsv
git commit -q -m "chore: ベンチ結果を記録する"
out="$(bash scripts/hooks/after-bash.sh <<<"$bench_json")"
check "PostToolUse の形で出す" "PostToolUse" "$(jq -r .hookSpecificOutput.hookEventName <<<"$out")"
contains "案内に design が入る" "docs/design/001_a.md" "$(ctx "$out")"
check "make prebench では何も出さない" "" "$(bash scripts/hooks/after-bash.sh <<<'{"tool_input":{"command":"make prebench"}}')"
check "文字列に含むだけなら何も出さない" "" "$(bash scripts/hooks/after-bash.sh <<<'{"tool_input":{"command":"echo \"make bench\""}}')"
check "関係のないコマンドでは何も出さない" "" "$(bash scripts/hooks/after-bash.sh <<<'{"tool_input":{"command":"ls"}}')"
finish_fixture

echo "after-bash（ベンチが記録を増やさなかったとき）"
make_fixture
write_doc . docs/design/001_a.md "design"
git add docs/design/001_a.md
git commit -q -m "docs: design"
check "not-benched なら何も出さない" "" "$(bash scripts/hooks/after-bash.sh <<<"$bench_json")"
finish_fixture

echo "before-stop"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/design/001_a.md "design"
git -C "$WT" add docs/design/001_a.md
git -C "$WT" commit -q -m "docs: design"
out="$(bash scripts/hooks/before-stop.sh <<<"$stop_json")"
check "Stop の形で出す" "Stop" "$(jq -r .hookSpecificOutput.hookEventName <<<"$out")"
contains "取り込みを促す" "make docs-merge" "$(ctx "$out")"
check "hooks で続けた後は何もしない" "" "$(bash scripts/hooks/before-stop.sh <<<'{"stop_hook_active":true,"background_tasks":[]}')"
check "裏で doc-writer が動いていれば何もしない" "" "$(bash scripts/hooks/before-stop.sh <<<'{"stop_hook_active":false,"background_tasks":[{"type":"subagent","agent_type":"doc-writer","status":"running"}]}')"
out="$(bash scripts/hooks/before-stop.sh <<<'{"stop_hook_active":false,"background_tasks":[{"type":"subagent","agent_type":"Explore","status":"running"}]}')"
contains "ほかのサブエージェントだけなら確かめる" "make docs-merge" "$(ctx "$out")"
check "background_tasks が無い入力でも動く" "Stop" "$(bash scripts/hooks/before-stop.sh <<<'{"stop_hook_active":false}' | jq -r .hookSpecificOutput.hookEventName)"
bash scripts/docs.sh merge >/dev/null 2>&1
check "取り込んだ後、ベンチ前の design だけなら何もしない" "" "$(bash scripts/hooks/before-stop.sh <<<"$stop_json")"
printf '20991231-000000\tabc\ttrue\t100\tm\t\n' >> docs/scores.tsv
git add docs/scores.tsv
git commit -q -m "chore: ベンチ結果を記録する"
contains "ベンチ済みで results が無ければ促す" "results を書かせ" "$(ctx "$(bash scripts/hooks/before-stop.sh <<<"$stop_json")")"
finish_fixture

echo "session-start"
make_fixture
out="$(bash scripts/hooks/session-start.sh <<<'{"source":"startup"}')"
contains "記録が無いとき" "まだ記録がありません" "$out"
contains "results が無いとき" "results はまだありません" "$out"
printf '20261030-110000\tabc\ttrue\t4242\tm\t\n' >> docs/scores.tsv
write_doc . docs/results/001_a.md "$(printf '# a\n\n## 次の候補\n\n- テンプレートを起動時に読む\n\n## 付録\n\n- これは出さない')"
git add docs
git commit -q -m "docs: results"
out="$(bash scripts/hooks/session-start.sh <<<'{"source":"clear"}')"
contains "ベストスコアを出す" "4242" "$out"
contains "最新の results のパスを出す" "docs/results/001_a.md" "$out"
contains "次の候補の中身を出す" "テンプレートを起動時に読む" "$out"
case "$out" in *"これは出さない"*) ng "次の候補の節だけを出す" ;; *) ok "次の候補の節だけを出す" ;; esac
check "10,000 文字以内" "yes" "$([ "${#out}" -le 10000 ] && echo yes || echo no)"
finish_fixture

report
