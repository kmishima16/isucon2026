#!/usr/bin/env bash
# scripts/lib.sh の deployed_rev と same_as_deployed を確かめる
source "$(dirname "$0")/helpers.sh"
same() { bash -c 'source scripts/lib.sh; if same_as_deployed; then echo same; else echo diff; fi'; }

echo "デプロイ済みとの比較"
make_fixture
mkdir -p logs
check "deploy 前は ? を返す" "?" "$(bash -c 'source scripts/lib.sh; deployed_rev')"
check "deploy 前は違う扱い" "diff" "$(same)"
echo "$(git rev-parse --short HEAD) 2026-10-30 10:00:00" > logs/deployed
check "同じコミットなら同じ" "same" "$(same)"
write_doc . docs/x.md "x"
git add docs/x.md
git commit -q -m "docs: x"
check "docs/ だけの差なら同じ" "same" "$(same)"
echo '# x' >> scripts/lint.sh
git add scripts/lint.sh
git commit -q -m "fix: x"
check "docs/ 以外の差なら違う" "diff" "$(same)"
finish_fixture

report
