#!/usr/bin/env bash
# Claude Code の hooks（SessionStart）。セッションの始めに、役割の確認・ベストスコア・持ち越しの issue の見方・docs の状態を読み込ませる。
# 標準出力がそのまま Claude の文脈に入る（上限 10,000 文字なので、各項目の行数を絞る）。通信はしない（gh も呼ばない）。手引きは docs/kit/hooks.md。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "# 今の状態（scripts/hooks/session-start.sh が読み込ませた）"
echo "決まりは CLAUDE.md。役割は人間が /isu-ops（操作役）・/isu-research（調査役）・/isu-local（検証役）のどれかで決める。決まっていなければ、サーバーを変える操作をせず人間に聞く。"
echo
echo "## ベストスコア（上位 5 件、docs/scores.tsv）"
rows=0
if [ -f docs/scores.tsv ]; then rows="$(wc -l < docs/scores.tsv)"; fi
if [ "$rows" -gt 1 ]; then make -s best N=5; else echo "まだ記録がありません"; fi
echo
echo "## 持ち越している改善項目"
echo "open の issue は gh issue list で見る（isu-ops の 3「候補を出す」で点検する）。"
echo
echo "## docs の状態"
n="$(bash scripts/docs.sh unmerged)"
if [ "$n" = 0 ]; then echo "docs ブランチはすべて main に入っています"; else echo "main に入っていない docs のコミットが ${n} 件あります。操作役なら make docs-merge で取り込む"; fi
