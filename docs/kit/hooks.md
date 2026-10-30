# hooks

Claude Code の hooks で、役割の確認と記録の状態を自動で読み込ませ、docs の取り込み漏れを確かめます。

## 役割

- .claude/settings.json の hooks で、scripts/hooks/ の 2 つを動かす。判定は scripts/docs.sh に任せ、hooks は整形と表示だけを受け持つ。
  - session-start.sh：セッションの開始・再開・/clear・compact・fork のときに、役割（/isu-ops か /isu-research）の確認・ベストスコアの上位 5 件・持ち越しの issue の見方（gh issue list）・docs の状態を読み込ませる。
  - before-stop.sh：作業を終える前に、main に入っていない docs（doc-writer の overview・retro）があれば、1 回だけ作業を続けさせる。裏で doc-writer が動いていれば何もしない。調査役の worktree でも同じ案内が出るが、取り込むのは操作役（make docs-merge は main でしか動かない）。
- 通信はしない（gh も呼ばない）。各 hook の上限は 10 秒。入力の JSON は jq で読む（macOS に入っている）。

## いつ使うか

- 自動で動く。人間も Claude も呼び出さない。

## 動かし方

- scripts/hooks/*.sh の冒頭のコメント。手で試すときは、見本の JSON を標準入力に渡す（tests/test_hooks.sh に例がある）。

## 確かめ方

- `make test`（tests/test_hooks.sh）。
- 開始時の hooks：/clear の直後に、役割の確認とベストスコアが文脈に入っている。
- 対話のセッションでは、フォルダの信頼の確認に同意するまで、hooks は動かない。
- hooks のスクリプトが失敗すると、エラーとして表示される（作業は止まらない）。

## 更新のしかた

- 何を読み込ませるか・いつ案内するか：scripts/hooks/*.sh。
- いつ動かすか：.claude/settings.json の hooks（matcher・timeout）。settings.json の hooks の変更は、通常はセッションの途中でも読み直される。
- 入出力の形：Claude Code の公式ドキュメント（code.claude.com/docs/en/hooks）。
- 直したら `make lint` と `make test`。
