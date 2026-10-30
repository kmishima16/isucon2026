#!/usr/bin/env bash
# 役割のスキル・レビュー役・issue のテンプレートが、hooks の案内と生きた文書に揃っていることを確かめる
source "$(dirname "$0")/helpers.sh"
cd "$REPO_SRC"

echo "役割のスキル"
for name in isu-ops isu-research; do
  f=".claude/skills/$name/SKILL.md"
  check "$name の SKILL.md がある" "yes" "$([ -f "$f" ] && echo yes || echo no)"
  check "$name の name が一致する" "name: $name" "$(grep -m1 '^name:' "$f" 2>/dev/null || true)"
  contains "session-start が /$name を案内する" "/$name" "$(cat scripts/hooks/session-start.sh)"
done

echo "レビュー役と issue のテンプレート"
check "reviewer がある" "name: reviewer" "$(grep -m1 '^name:' .claude/agents/reviewer.md 2>/dev/null || true)"
check "テンプレートに 4 つの見出しがある" "4" "$(grep -cE '^## (経緯|わかったこと|原因|完了の条件)$' .github/ISSUE_TEMPLATE/kaizen.md 2>/dev/null || true)"

echo "生きた文書"
check "古い手順（出来事ごとの手順）への参照が無い" "" "$(grep -rln '出来事ごとの手順' README.md CLAUDE.md docs/README.md docs/kit docs/flow .claude/agents .claude/skills 2>/dev/null || true)"

report
