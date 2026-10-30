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

has() {   # 説明 ファイル 含むはずの固定文字列
  if grep -qF -- "$3" "$2"; then ok "$1"; else ng "$1"; printf '      %s に「%s」がありません\n' "$2" "$3"; fi
}
lacks() {   # 説明 ファイル 含まないはずの固定文字列
  if grep -qF -- "$3" "$2"; then ng "$1"; printf '      %s に「%s」が残っています\n' "$2" "$3"; else ok "$1"; fi
}

echo "本番（ポータルからベンチ）の流れ"
has "isu-ops がポータルの結果の記録を案内する" .claude/skills/isu-ops/SKILL.md 'make portal WATCH=1'
has "isu-ops がポータルのベンチ中は deploy しないと書く" .claude/skills/isu-ops/SKILL.md 'ポータルのベンチが終わるまで deploy しない'
has "止まる条件にポータルのベンチを頼むときがある" .claude/skills/isu-ops/SKILL.md 'ポータルのベンチを人間に頼む'
has "bench.sh の案内が make portal を勧める" scripts/bench.sh 'make portal WATCH=1'

echo "調査役の下ごしらえの取り込みと、作業ツリーの持ち主"
has "isu-ops は下ごしらえを cherry-pick で載せる" .claude/skills/isu-ops/SKILL.md 'git cherry-pick'
lacks "isu-ops は ff-only の取り込みを前提にしない" .claude/skills/isu-ops/SKILL.md 'merge --ff-only'
has "isu-research はコミットの SHA を伝える" .claude/skills/isu-research/SKILL.md 'コミットの SHA'
lacks "CLAUDE.md に、どの役でも道具を直してよいが残っていない" CLAUDE.md 'どの役でもしてよい'
has "CLAUDE.md が main の作業ツリーの持ち主を書く" CLAUDE.md 'main のチェックアウトの作業ツリーを書き換えるのは操作役だけ'

echo "調査役の始め方と gh"
has "isu-research が最初に状態を集める" .claude/skills/isu-research/SKILL.md 'gh issue list'
has "isu-research に gh が失敗したときの扱いがある" .claude/skills/isu-research/SKILL.md 'gh が失敗したら'

echo "issue の close と削除"
has "CLAUDE.md が close の証拠をコメントに残させる" CLAUDE.md '証拠をコメントに書いて close'
has "settings.json が gh issue delete を断る" .claude/settings.json 'Bash(gh issue delete *)'
has "settings.json が gh issue transfer を断る" .claude/settings.json 'Bash(gh issue transfer *)'

echo "実行中に書くファイルを deploy で消さない"
has "isu-ops がファイルに書く施策で APP_EXCLUDE を足させる" .claude/skills/isu-ops/SKILL.md 'APP_EXCLUDE'
has "reviewer が deploy で消えないかを見る" .claude/agents/reviewer.md 'APP_EXCLUDE'
has "deploy の手引きが rsync --delete と APP_EXCLUDE を書く" docs/kit/deploy.md '`APP_EXCLUDE` に足す'

echo "本番の時間割"
has "contest.md に初期データの退避がある" docs/flow/contest.md 'mysqldump'
has "contest.md に最終のデータの確認がある" docs/flow/contest.md 'データが残っていることを確かめる'

report
