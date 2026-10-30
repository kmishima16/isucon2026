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

echo "fail の件数"
make_fixture
rm docs/scores.tsv
S=10 F=3 bash scripts/bench.sh score >/dev/null 2>&1
check "新しい記録の見出しに fail がある" "$(printf 'time\trev\tpass\tscore\tmemo\texpect\tfail')" "$(head -1 docs/scores.tsv)"
check "make score の F= が 7 列目に入る" "3" "$(tail -1 docs/scores.tsv | cut -f7)"
printf '{"pass":true,"score":5,"fail":2,"messages":[]}\n' > portal.json
printf "PORTAL_RESULT_URL=''\nPORTAL_RESULT_CMD='cat portal.json'\n" >> isu.env
bash scripts/portal.sh >/dev/null 2>&1
check "portal の fail が 7 列目に入る" "$(printf '5\t2')" "$(tail -1 docs/scores.tsv | cut -f4,7)"
finish_fixture

echo "make best"
make_fixture
{
  printf '20261030-100000\taaa\ttrue\t300\tfail あり\t\t643\n'
  printf '20261030-100100\tbbb\ttrue\t200\tfail なし\t\t0\n'
  printf '20261030-100200\tccc\tfalse\t250\tpass でない\t\t0\n'
  printf '20261030-100300\tddd\ttrue\t100\t古い記録\t\n'
} >> docs/scores.tsv
out="$(make -s best)"
case "$out" in *"aaa"*) ng "fail のある行を出さない" ;; *) ok "fail のある行を出さない" ;; esac
case "$out" in *"ccc"*) ng "pass でない行を出さない" ;; *) ok "pass でない行を出さない" ;; esac
check "fail の無い行が先頭に来る" "bbb" "$(head -1 <<<"$out" | awk '{print $2}')"
contains "fail が分からない古い行は ? と出して残す" "fail=?" "$(grep ddd <<<"$out")"
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
