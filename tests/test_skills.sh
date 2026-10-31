#!/usr/bin/env bash
# 役割のスキル・レビュー役・issue のテンプレートが、hooks の案内と生きた文書に揃っていることを確かめる
source "$(dirname "$0")/helpers.sh"
cd "$REPO_SRC"

echo "役割のスキル"
for name in isu-ops isu-research isu-local; do
  f=".claude/skills/$name/SKILL.md"
  check "$name の SKILL.md がある" "yes" "$([ -f "$f" ] && echo yes || echo no)"
  check "$name の name が一致する" "name: $name" "$(grep -m1 '^name:' "$f" 2>/dev/null || true)"
  contains "session-start が /$name を案内する" "/$name" "$(cat scripts/hooks/session-start.sh)"
done

echo "レビュー役と issue のテンプレート"
check "reviewer がある" "name: reviewer" "$(grep -m1 '^name:' .claude/agents/reviewer.md 2>/dev/null || true)"
check "テンプレートに 5 つの見出しがある" "5" "$(grep -cE '^## (経緯|わかったこと|原因|完了の条件|閉じる条件)$' .github/ISSUE_TEMPLATE/kaizen.md 2>/dev/null || true)"

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
has "bench.sh の案内は本番のスコアを make score で記録させる" scripts/bench.sh 'make score S=<点数>'

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

echo "消した手順への参照（Makefile の help と scripts も見る）"
check "results や消した手順への参照が無い" "" "$(grep -rlE 'results の無い design|results を書かせ|最新の results|手順 7 の判断|施策の回し方の 1' README.md CLAUDE.md Makefile docs/README.md docs/kit docs/flow .claude/skills .claude/agents scripts 2>/dev/null || true)"
has "make test の help がテストの中身を挙げる" Makefile 'スキルの揃い'
has "session-start の docs-merge の案内は操作役向け" scripts/hooks/session-start.sh '操作役なら make docs-merge'
has "session-start は点検をスキルの 3 と案内する" scripts/hooks/session-start.sh 'isu-ops の 3'

echo "CLAUDE.md とスキルの抜け"
has "伝え方にベンチの出力の場所がある" CLAUDE.md 'logs/<時刻>-bench.txt'
has "道具が失敗したら手引きを読む" CLAUDE.md 'docs/kit/<道具>.md'
has "isu-ops が移行の間は results の次の候補も読む" .claude/skills/isu-ops/SKILL.md 'docs/results'
has "isu-ops の最初の一周で回帰チェックの基準を取る" .claude/skills/isu-ops/SKILL.md '`make analyze` → `make snapshot`'
has "first-time の一周で回帰チェックの基準を取る" docs/flow/first-time.md 'make snapshot'
lacks "first-time に origin を二重に足す手順が無い" docs/flow/first-time.md 'git remote add origin'

echo "doc-writer の同時書き込み"
has "doc-writer が書く前に書きかけを確かめる" .claude/agents/doc-writer.md 'status --porcelain'

echo "snapshot の初期化とポータルのベンチ"
has "isu-ops がポータルのベンチ中に snapshot を取らせない" .claude/skills/isu-ops/SKILL.md 'ポータルのベンチが終わってから'
has "snapshot の手引きがベンチとぶつかることを書く" docs/kit/snapshot.md 'ポータルのベンチが終わってから'
has "first-time が snapshot の時期を書く" docs/flow/first-time.md 'ポータルのベンチが終わってから'

echo "本番の手順"
has "contest.md に初期データの退避がある" docs/flow/contest.md 'mysqldump'
has "contest.md に最終のデータの確認がある" docs/flow/contest.md 'データが残っていることを確かめる'
lacks "contest.md に時刻の時間割が無い（時刻のガイドは人間向けに別に置く）" docs/flow/contest.md '10:00'
has "contest.md のスコアの記録は make score が既定" docs/flow/contest.md 'make score S=<点数>` を既定'
has "contest.md に道具が使えないときの置き換えがある" docs/flow/contest.md '道具が使えないとき'
has "portal の手引きが本番の既定を make score と書く" docs/kit/portal.md 'make score S=<点数>` で記録するのを既定'

echo "立ち上げ（環境が立ってから、人間が最初の施策を選ぶまで）"
has "isu-ops に立ち上げの手順がある" .claude/skills/isu-ops/SKILL.md '## 立ち上げ'
has "isu-ops は人間を待たずにバイナリログを止める" .claude/skills/isu-ops/SKILL.md 'disable_log_bin'
has "isu-ops は人間を待たずに innodb_flush_log_at_trx_commit を 2 にする" .claude/skills/isu-ops/SKILL.md 'innodb_flush_log_at_trx_commit=2'
has "isu-ops の立ち上げで初期データを退避する" .claude/skills/isu-ops/SKILL.md 'make local-seed'
has "isu-ops は本番のスコアを make score で記録する" .claude/skills/isu-ops/SKILL.md 'make score S=<点数>'
has "isu-research は取り込みのコミットを見てコードを読み始める" .claude/skills/isu-research/SKILL.md 'chore: 初期アプリを取り込む'
has "isu-local は立てる手間に上限がある" .claude/skills/isu-local/SKILL.md '30 分'
has "isu-local は定期的な呼び出しを取りこぼさない" .claude/skills/isu-local/SKILL.md 'setInterval'
has "isu-local はサーバーに ssh しない" .claude/skills/isu-local/SKILL.md 'ssh しない'
has "CLAUDE.md に検証役がある" CLAUDE.md '/isu-local'
has "CLAUDE.md にキットを直してよい条件がある" CLAUDE.md '10 分ほどで直せる'
has "CLAUDE.md にメッセージが届かないときの扱いがある" CLAUDE.md 'メッセージは届かないことがある'
has "README から検証役のスキルを辿れる" README.md '.claude/skills/isu-local/SKILL.md'

echo "レビューの指摘（立ち上げの食い違いと抜け）"
has "CLAUDE.md が立ち上げの 3 つの施策を、人間の判断の例外として書く" CLAUDE.md '立ち上げの 3 つの施策'
has "CLAUDE.md が本番中のキットの気付きを控えに残させる" CLAUDE.md 'logs/kit-notes.md'
has "isu-ops はアプリを取り込んだコミットの文言を決める" .claude/skills/isu-ops/SKILL.md 'chore: 初期アプリを取り込む'
has "isu-ops は初期データの退避のコミットの文言を決める" .claude/skills/isu-ops/SKILL.md 'chore: 初期データを手元に退避する'
has "isu-ops はインデックスの前に初期化の SQL を確かめる" .claude/skills/isu-ops/SKILL.md '初期化の SQL'
has "isu-ops は本番では 3 つの施策のベンチを 1 回にまとめる" .claude/skills/isu-ops/SKILL.md 'ポータルのベンチは 1 回だけ'
has "isu-research は main の git log を見る" .claude/skills/isu-research/SKILL.md 'git log main'
has "isu-research は検証役の対応表のファイルを見る" .claude/skills/isu-research/SKILL.md 'map.md'
has "isu-local は main の git log を見る" .claude/skills/isu-local/SKILL.md 'git log main'
has "isu-local は対応表をファイルに書く" .claude/skills/isu-local/SKILL.md 'map.md'
has "isu-local はほかの環境とのぶつかりを docker ps で見る" .claude/skills/isu-local/SKILL.md 'docker ps'
has "isu-local はアプリの宛先の変数を書く" .claude/skills/isu-local/SKILL.md 'LOCAL_UPSTREAM'
lacks "isu-local に、worktree なら道具がすべて断るという誤りが無い" .claude/skills/isu-local/SKILL.md '道具も断る'
has "contest.md が fail のときに PASS=false を付けさせる" docs/flow/contest.md 'PASS=false'
lacks "bench の手引きが本番で make portal を先に勧めない" docs/kit/bench.md 'ポータルでベンチ → `make portal`'
lacks "portal の手引きが本番当日に URL を埋めさせない" docs/kit/portal.md '本番当日：ポータルを開発者ツールで見て'
has "doc-writer が仕様メモの画像もコミットする" .claude/agents/doc-writer.md '画像'
has "docs/README が overview の画像の置き場を決める" docs/README.md '画像'
lacks "first-time が古い zip からキットを取り出さない" docs/flow/first-time.md 'unzip isucon-kit.zip'
has "first-time がキットを履歴ごと取り込み、更新を取り込めるようにする" docs/flow/first-time.md 'git remote rename origin kit'

echo "道具は isucon-kit で直す"
has "CLAUDE.md が道具を isucon-kit で直させる" CLAUDE.md '道具は isucon-kit で直す'
has "CLAUDE.md がキットの更新の取り込み方を書く" CLAUDE.md 'git fetch kit && git merge kit/main'
lacks "CLAUDE.md に、練習で直してから移すという古い流れが無い" CLAUDE.md '練習用のリポジトリで確かめたら、isucon-kit に移す'

echo "造語を使わない（ふつうの言葉で書く）"
check "スキル・CLAUDE.md・手引きに造語が無い" "" "$(grep -rlE 'クリティカルパス|砂場|合図|しるし|待つもの|知らせ' CLAUDE.md README.md .claude/skills .claude/agents docs/flow docs/kit scripts/hooks 2>/dev/null || true)"

report
