#!/usr/bin/env bash
# scripts/docs.sh の pending・unmerged・merge を確かめる
source "$(dirname "$0")/helpers.sh"
WT=.claude/worktrees/docs

echo "docs ブランチが無いとき"
make_fixture
check "pending は何も出さない" "" "$(bash scripts/docs.sh pending)"
check "unmerged は 0" "0" "$(bash scripts/docs.sh unmerged)"
if bash scripts/docs.sh merge >/dev/null 2>&1; then ok "merge は何もせず通る"; else ng "merge は何もせず通る"; fi
finish_fixture

echo "docs ブランチの design と、その後のベンチの記録"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/design/001_a.md "design"
git -C "$WT" add docs/design/001_a.md
commit_at "$WT" "2026-10-30 10:00:00" "docs: design"
check "記録が無ければ not-benched" "$(printf 'docs/design/001_a.md\tnot-benched')" "$(bash scripts/docs.sh pending)"
printf '20261030-090000\tabc\ttrue\t100\tm\t\n' >> docs/scores.tsv
git add docs/scores.tsv
commit_at . "2026-10-30 09:00:00" "chore: ベンチ結果を記録する"
check "design より前の記録なら not-benched" "$(printf 'docs/design/001_a.md\tnot-benched')" "$(bash scripts/docs.sh pending)"
printf '20261030-110000\tabc\ttrue\t200\tm\t\n' >> docs/scores.tsv
git add docs/scores.tsv
commit_at . "2026-10-30 11:00:00" "chore: ベンチ結果を記録する"
check "design より後の記録なら benched" "$(printf 'docs/design/001_a.md\tbenched')" "$(bash scripts/docs.sh pending)"
check "unmerged は 1" "1" "$(bash scripts/docs.sh unmerged)"
write_doc "$WT" docs/results/001_a.md "results"
git -C "$WT" add docs/results/001_a.md
git -C "$WT" commit -q -m "docs: results"
check "results があれば pending に出ない" "" "$(bash scripts/docs.sh pending)"
finish_fixture

echo "merge：早送りできるとき"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/design/001_a.md "design"
git -C "$WT" add docs/design/001_a.md
git -C "$WT" commit -q -m "docs: design"
bash scripts/docs.sh merge >/dev/null 2>&1
check "main が docs と同じになる" "$(git rev-parse docs)" "$(git rev-parse main)"
check "unmerged は 0" "0" "$(bash scripts/docs.sh unmerged)"
finish_fixture

echo "merge：main が先に進んでいるとき"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/design/001_a.md "design"
git -C "$WT" add docs/design/001_a.md
git -C "$WT" commit -q -m "docs: design"
printf '20261030-110000\tabc\ttrue\t200\tm\t\n' >> docs/scores.tsv
git add docs/scores.tsv
git commit -q -m "chore: ベンチ結果を記録する"
bash scripts/docs.sh merge >/dev/null 2>&1
check "unmerged は 0" "0" "$(bash scripts/docs.sh unmerged)"
check "main に design が入る" "yes" "$(git cat-file -e main:docs/design/001_a.md 2>/dev/null && echo yes || echo no)"
finish_fixture

echo "merge：main を取り込むと衝突するとき"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/design/001_a.md "docs 側"
git -C "$WT" add docs/design/001_a.md
git -C "$WT" commit -q -m "docs: design"
write_doc . docs/design/001_a.md "main 側"
git add docs/design/001_a.md
git commit -q -m "docs: 衝突させる"
before="$(git rev-parse HEAD)"
if bash scripts/docs.sh merge >/dev/null 2>&1; then ng "失敗で終わる"; else ok "失敗で終わる"; fi
check "main は動かない" "$before" "$(git rev-parse HEAD)"
check "worktree に取り込みの途中が残らない" "no" "$(git -C "$WT" rev-parse -q --verify MERGE_HEAD >/dev/null && echo yes || echo no)"
finish_fixture

echo "merge：main 以外のチェックアウトでは止まる"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/design/001_a.md "design"
git -C "$WT" add docs/design/001_a.md
git -C "$WT" commit -q -m "docs: design"
git checkout -q -b other
if bash scripts/docs.sh merge >/dev/null 2>&1; then ng "失敗で終わる"; else ok "失敗で終わる"; fi
finish_fixture

report
