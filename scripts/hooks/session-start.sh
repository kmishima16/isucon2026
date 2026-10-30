#!/usr/bin/env bash
# Claude Code の hooks（SessionStart）。セッションの始めに、ベストスコア・最新の results の「次の候補」・docs の状態を読み込ませる。
# 標準出力がそのまま Claude の文脈に入る（上限 10,000 文字なので、各項目の行数を絞る）。通信はしない。手引きは docs/kit/hooks.md。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "# 今の状態（scripts/hooks/session-start.sh が読み込ませた）"
echo "手順は CLAUDE.md の「出来事ごとの手順」、道具の手引きは README.md の目次から辿る。"
echo
echo "## ベストスコア（上位 5 件、docs/scores.tsv）"
rows=0
if [ -f docs/scores.tsv ]; then rows="$(wc -l < docs/scores.tsv)"; fi
if [ "$rows" -gt 1 ]; then make -s best N=5; else echo "まだ記録がありません"; fi
echo
echo "## 最新の results"
latest=""
for f in docs/results/[0-9][0-9][0-9]_*.md; do
  if [ -e "$f" ]; then latest="$f"; fi
done
if [ -n "$latest" ]; then
  echo "$latest の「次の候補」："
  awk '/^## 次の候補/ { on = 1; next } /^## / { on = 0 } on' "$latest" | head -n 40
else
  echo "results はまだありません（最初の施策は make analyze の結果から選ぶ）"
fi
echo
echo "## docs の状態"
pending="$(bash scripts/docs.sh pending)"
if [ -n "$pending" ]; then
  echo "results の無い design（benched はベンチ済みで results を書かせる番、not-benched は実装中）："
  printf '%s\n' "$pending" | sed 's/^/- /'
else
  echo "results の無い design はありません"
fi
n="$(bash scripts/docs.sh unmerged)"
if [ "$n" = 0 ]; then echo "docs ブランチはすべて main に入っています"; else echo "main に入っていない docs のコミットが ${n} 件あります。make docs-merge で取り込む"; fi
