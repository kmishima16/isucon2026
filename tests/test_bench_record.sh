#!/usr/bin/env bash
# scripts/bench.sh の record（make score・portal・bench が通る）を確かめる
source "$(dirname "$0")/helpers.sh"

echo "記録とコミット"
make_fixture
echo x > other.txt
git add other.txt   # ステージしてあるほかの変更が巻き込まれないことを見るため
S=1234 M=memo E=exp bash scripts/bench.sh score >/dev/null 2>&1
check "最後の行に記録される" "$(printf 'true\t1234\tmemo\texp')" "$(tail -1 docs/scores.tsv | cut -f3-6)"
check "コミットの件名" "chore: ベンチ結果を記録する" "$(git log -1 --format=%s)"
check "記録だけがコミットされる" "docs/scores.tsv" "$(git diff-tree --no-commit-id --name-only -r HEAD)"
check "ほかの変更はステージされたまま残る" "A  other.txt" "$(git status --porcelain other.txt)"
check "記録は未コミットで残らない" "" "$(git status --porcelain docs/scores.tsv)"
finish_fixture

report
