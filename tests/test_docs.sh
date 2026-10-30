#!/usr/bin/env bash
# scripts/docs.sh の unmerged・merge を確かめる
source "$(dirname "$0")/helpers.sh"
WT=.claude/worktrees/docs

echo "docs ブランチが無いとき"
make_fixture
check "unmerged は 0" "0" "$(bash scripts/docs.sh unmerged)"
if bash scripts/docs.sh merge >/dev/null 2>&1; then ok "merge は何もせず通る"; else ng "merge は何もせず通る"; fi
finish_fixture

echo "docs ブランチにコミットがあるとき"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/overview/001_a.md "overview"
git -C "$WT" add docs/overview/001_a.md
git -C "$WT" commit -q -m "docs: overview"
check "unmerged は 1" "1" "$(bash scripts/docs.sh unmerged)"
contains "status は取り込みを促す" "make docs-merge" "$(bash scripts/docs.sh status 2>&1)"
finish_fixture

echo "merge：早送りできるとき"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/overview/001_a.md "overview"
git -C "$WT" add docs/overview/001_a.md
git -C "$WT" commit -q -m "docs: overview"
bash scripts/docs.sh merge >/dev/null 2>&1
check "main が docs と同じになる" "$(git rev-parse docs)" "$(git rev-parse main)"
check "unmerged は 0" "0" "$(bash scripts/docs.sh unmerged)"
finish_fixture

echo "merge：main が先に進んでいるとき"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/overview/001_a.md "overview"
git -C "$WT" add docs/overview/001_a.md
git -C "$WT" commit -q -m "docs: overview"
printf '20261030-110000\tabc\ttrue\t200\tm\t\n' >> docs/scores.tsv
git add docs/scores.tsv
git commit -q -m "chore: ベンチ結果を記録する"
bash scripts/docs.sh merge >/dev/null 2>&1
check "unmerged は 0" "0" "$(bash scripts/docs.sh unmerged)"
check "main に overview が入る" "yes" "$(git cat-file -e main:docs/overview/001_a.md 2>/dev/null && echo yes || echo no)"
finish_fixture

echo "merge：main を取り込むと衝突するとき"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/overview/001_a.md "docs 側"
git -C "$WT" add docs/overview/001_a.md
git -C "$WT" commit -q -m "docs: overview"
write_doc . docs/overview/001_a.md "main 側"
git add docs/overview/001_a.md
git commit -q -m "docs: 衝突させる"
before="$(git rev-parse HEAD)"
if bash scripts/docs.sh merge >/dev/null 2>&1; then ng "失敗で終わる"; else ok "失敗で終わる"; fi
check "main は動かない" "$before" "$(git rev-parse HEAD)"
check "worktree に取り込みの途中が残らない" "no" "$(git -C "$WT" rev-parse -q --verify MERGE_HEAD >/dev/null && echo yes || echo no)"
finish_fixture

echo "merge：main 以外のチェックアウトでは止まる"
make_fixture
git worktree add -q -b docs "$WT" main
write_doc "$WT" docs/overview/001_a.md "overview"
git -C "$WT" add docs/overview/001_a.md
git -C "$WT" commit -q -m "docs: overview"
git checkout -q -b other
if bash scripts/docs.sh merge >/dev/null 2>&1; then ng "失敗で終わる"; else ok "失敗で終わる"; fi
finish_fixture

echo "main ブランチが無いとき（git init の既定が master のリポジトリ）"
make_fixture
git branch -q -m main master
if out="$(bash scripts/docs.sh unmerged 2>&1)"; then ng "unmerged は失敗で終わる"; else contains "unmerged は失敗で終わり、理由を出す" "main ブランチがありません" "$out"; fi
finish_fixture

report
