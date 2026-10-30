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

echo "worktree ではロックを取らない"
make_fixture
git worktree add -q -b wt .claude/worktrees/wt main
if out="$(cd .claude/worktrees/wt && bash -c 'source scripts/lib.sh; acquire_lock test' 2>&1)"; then
  ng "worktree では失敗で終わる"
else
  contains "worktree では理由を出して止まる" "main のチェックアウトで" "$out"
fi
contains "main のチェックアウトではロックを取れる" "got" "$(bash -c 'source scripts/lib.sh; acquire_lock test && echo got' 2>&1)"
finish_fixture

echo "壊れた worktree（本体を動かした・prune した）でも断る"
make_fixture
git worktree add -q -b wt .claude/worktrees/wt main
echo "gitdir: /nonexistent/.git/worktrees/wt" > .claude/worktrees/wt/.git
if out="$(cd .claude/worktrees/wt && bash -c 'source scripts/lib.sh; acquire_lock test' 2>&1)"; then
  ng "壊れた worktree では失敗で終わる"
else
  contains "壊れた worktree でも理由を出して止まる" "main のチェックアウトで" "$out"
fi
finish_fixture

echo "worktree では、ロックを取らない入口（計測の切り替え・prebench・記録・setup・agent）も断る"
make_fixture
# 断られなかったときに本物のサーバーへ届かないよう、宛先を存在しない名前にする
printf 'HOSTS=no-such-host.invalid\nAPP_HOSTS=$HOSTS\nNGINX_HOST=$HOSTS\nDB_HOST=$HOSTS\nPPROF_HOST=$HOSTS\n' >> isu.env
git commit -qam "test: 宛先を外す"
git worktree add -q -b wt .claude/worktrees/wt main
cd .claude/worktrees/wt
head="$(git rev-parse HEAD)"
for cmd in "measure.sh off" "measure.sh on" "bench.sh prebench" "bench.sh score" "portal.sh" "setup.sh" "pprotein.sh agent-off" "pprotein.sh agent-on"; do
  # shellcheck disable=SC2086
  if out="$(S=1 bash scripts/$cmd 2>&1)"; then
    ng "$cmd は失敗で終わる"
  else
    contains "$cmd は理由を出して止まる" "main のチェックアウトで" "$out"
  fi
done
check "worktree のブランチにコミットが増えない" "$head" "$(git rev-parse HEAD)"
finish_fixture

report
