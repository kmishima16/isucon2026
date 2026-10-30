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

echo "次の一手"
make_fixture
out="$(S=1 bash scripts/bench.sh score 2>&1)"
contains "design が無いとき" "対応する design はありません" "$out"
write_doc . docs/design/001_a.md "design"
git add docs/design/001_a.md
commit_at . "2020-01-01 00:00:00" "docs: design"
out="$(S=2 bash scripts/bench.sh score 2>&1)"
contains "ベンチ済みの design があるとき" "results を書かせる（docs/design/001_a.md）" "$out"
finish_fixture

echo "記録のコミットに失敗したとき"
make_fixture
printf '#!/bin/sh\nexit 1\n' > .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
if S=1 bash scripts/bench.sh score >/dev/null 2>&1; then ng "score は失敗で終わる"; else ok "score は失敗で終わる"; fi
printf '{"pass":true,"score":5,"messages":[]}\n' > portal.json
printf "PORTAL_RESULT_URL=''\nPORTAL_RESULT_CMD='cat portal.json'\n" >> isu.env
if bash scripts/portal.sh >/dev/null 2>&1; then ng "portal は失敗で終わる"; else ok "portal は失敗で終わる"; fi
check "portal は記録済みにしない" "" "$(cat logs/portal-seen 2>/dev/null)"
finish_fixture

report
