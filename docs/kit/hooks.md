# hooks

Claude Code の hooks で、記録と docs の状態を自動で読み込ませ、取りこぼしを確かめます。

## 役割

- .claude/settings.json の hooks で、scripts/hooks/ の 3 つを動かす。判定は scripts/docs.sh に任せ、hooks は整形と表示だけを受け持つ。
  - session-start.sh：セッションの開始・再開・/clear・compact・fork のときに、ベストスコアの上位 5 件・最新の results の「次の候補」・docs の状態を読み込ませる。
  - after-bash.sh：make bench・score・portal の後に、ベンチ済みで results の無い design があれば、results を書かせるよう案内する。成功したかは、終了コードではなく記録で判断する。
  - before-stop.sh：作業を終える前に、main に入っていない docs と、ベンチ済みで results の無い design があれば、1 回だけ作業を続けさせる。裏で doc-writer が動いていれば何もしない。
  - 判定はリポジトリ全体の状態で行い、裏の doc-writer はそのセッションの分しか見えない。施策を進めるセッションは 1 つにする前提で、ほかのセッションにも同じ案内が出ることがある（CLAUDE.md の「文書の扱い」）。
- 通信はしない。各 hook の上限は 10 秒。入力の JSON は jq で読む（macOS に入っている）。

## いつ使うか

- 自動で動く。人間も Claude も呼び出さない。

## 動かし方

- scripts/hooks/*.sh の冒頭のコメント。手で試すときは、見本の JSON を標準入力に渡す（tests/test_hooks.sh に例がある）。

## 確かめ方

- `make test`（tests/test_hooks.sh）。
- 開始時の hooks：/clear の直後に、ベストスコアと次の候補が文脈に入っている。
- 対話のセッションでは、フォルダの信頼の確認に同意するまで、hooks は動かない。
- hooks のスクリプトが失敗すると、エラーとして表示される（作業は止まらない）。

## 更新のしかた

- 何を読み込ませるか・いつ案内するか：scripts/hooks/*.sh。
- いつ動かすか：.claude/settings.json の hooks（matcher・timeout）。settings.json の hooks の変更は、通常はセッションの途中でも読み直される。
- 入出力の形：Claude Code の公式ドキュメント（code.claude.com/docs/en/hooks）。
- 直したら `make lint` と `make test`。
