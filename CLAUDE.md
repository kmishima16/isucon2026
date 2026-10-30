# CLAUDE.md

Webアプリのパフォーマンスチューニング（ISUCON形式）を、人間1人と Claude で行うリポジトリ。

## 前提

- 文書の目次は `README.md`。道具ごとの手引きは `docs/kit/`、場面ごとの手順は `docs/flow/`、docs の規則は `docs/README.md`。
- サーバー構成・アプリの場所・ベンチの方法は `isu.env`。道具の既定値は `scripts/lib.sh`。操作は `make help`。
- **このリポジトリが正**。サーバーへの反映は `git commit` → `make deploy` だけで行う。
- サーバーに置く設定ファイルは `server/all/<絶対パス>`（全台）か `server/<ホスト名>/<絶対パス>`（その台だけ）。既存の設定は `make pull F=/etc/...` で取り込んでから編集する。DB のスキーマ変更は `sql/NNN_*.sql`（何度流しても壊れない形で）。
- 手元だけの環境変数は `.envrc`（direnv）。Claude の Bash では `direnv exec . make ...` で実行する。`make up` / `make down` は人間だけ。
- 計測の結果は `logs/`（make analyze）と `pprotein/data/`、ベンチの記録は `docs/scores.tsv`。

## 役割

次の施策を決めるのは人間。LLM の仕事は、全体像を素早くまとめて伝えること・実装・道具での調査・deploy の 4 つ。すべてを書き残して管理することはしない。

- 操作役：人間が `/isu-ops` と打ったセッション（`.claude/skills/isu-ops/SKILL.md`）。deploy・bench・analyze・reboot-test・measure-* を実行するのは、この役だけ。main のチェックアウトで動く（worktree では、サーバーや記録を変える道具が断る）。
- 調査役：人間が `/isu-research` と打ったセッション（`.claude/skills/isu-research/SKILL.md`）。ssh は読み取りだけ。コードを変えるのは自分の worktree の中だけ。
- レビュー役：サブエージェント reviewer（`.claude/agents/reviewer.md`）。操作役が施策ごとに diff を渡す。
- 文書：サブエージェント doc-writer（`.claude/agents/doc-writer.md`）。overview と retro だけを書く。返ったら操作役が `make docs-merge`。
- どちらのスキルも打たれていないセッションは、サーバーを変える操作をしない。
- main のチェックアウトの作業ツリーを書き換えるのは操作役だけ。ほかの役がコードや道具を直すときは、自分の worktree で 1 コミットにし、操作役が `git cherry-pick` で取り込む（混ざると、ベンチで効果を切り分けられない・deploy が止まる）。

## 守ること

1. **最終状態は `make deploy` で再現できること。** サーバー上で直接変えたものは、取り込む（`make pull`）か元に戻す。
2. **deploy の前にコミットする。** ベンチ結果はコミットに紐づけて記録される。
3. **次は触らない**（壊すと外から入れなくなり、復旧できない）：ssh / sshd の設定と停止、ネットワーク設定（netplan・IP・ルーティング・MTU・resolv.conf・hosts）、ファイアウォール（ufw・iptables・nftables）、AWS（aws CLI、isuenv up/down）、外から見える 80/443 の待ち受けを止めること。`git push` も人間が行う。
4. **失敗を握りつぶさない。** `|| true` やエラーの無視でビルド・設定チェックを通さない。
5. **レギュレーション**：再起動後も動く／書き込んだデータが再起動後も残る／`/initialize` が動く／ベンチ中の処理を外部に委譲しない。
6. **取り返しのつきにくい変更の前は、人間に確かめる。** インスタンスやベンチ台・台の役割の変更、DNS の向き先、データの削除。
7. **ベンチが fail したら、次のベンチを回さない。** 原因を見るか、戻す。

## 伝え方

チャットでは：

- 何をなぜ変え、どうなったかが分かればよい。報告の形は決めない。
- 原因や提案には、人間が見に行ける場所を添える。計測は logs/ のファイルと行、コードは file:line、変更はコミットの SHA、グラフは pprotein（`make pp-up`、http://localhost:9000）。推測には「推測」と書く。

issue では（GitHub から読むので、手元のパスや行番号は見えない）：

- テンプレート `.github/ISSUE_TEMPLATE/kaizen.md`（経緯・わかったこと・原因・完了の条件）を写して埋め、frontmatter（`---` の間）と HTML コメントを除いた本文を `gh issue create --body-file` に渡す。
- 根拠は抜粋を貼る。計測は該当する行を、取った時刻とコミット付きで。コードは 10〜20 行を、ファイル名と関数名付きで。
- 構造を説明する（どのリクエストがどの処理を何回呼ぶか、どのデータをどこに持つか）。図は Mermaid。
- 一般的なシステムの用語は使ってよい。このリポジトリだけの言葉（make のターゲット名・役割名・scores.tsv など）は避けるか、一言の説明を添える。

## issue の決まり

- 立てるのは、今はやらないが持ち越すものだけ（人間が残すと決めた候補、今は直さないレビューの指摘）。その場でやる施策は立てない。
- open は 10 件まで。超えたら、立てる前に整理を提案する。
- 操作役は、候補を出すたびに open の issue を点検する。完了の条件を満たしたものは、証拠を付けて close を提案し、人間の OK が出たら証拠をコメントに書いて close する（`gh issue close <N> --comment ...`）。2 回続けて案に挙げて選ばれなかったものは、not planned での close を提案する。
- ラベルは使わない。キットの改善は isucon-kit のリポジトリに立てる。

## コミット

- 施策ごとに 1 コミット。複数の施策を混ぜてからまとめてコミットしない（ベンチで効果を切り分けられなくなる）。
- `prefix: 何を変えたか` の 1 行で書く。「〜するため」で終わる理由だけの書き方はしない。フッター（Co-Authored-By など）は付けない。issue に当たる施策なら末尾に `(#N)` を付ける。
  - 例：`perf: 画像を nginx から静的配信する`、`fix: analyze の alp 出力項目を 1.0 系に合わせる`
- prefix：`perf`（性能改善）・`fix`（不具合の修正）・`feat`（機能や道具の追加）・`refactor`（動きを変えない整理）・`chore`（記録・雑務）・`docs`（文書）。
- 記録は道具がその場でコミットする（`chore: ベンチ結果を記録する`、`chore: 計測を外す`／`chore: 計測を戻す`）。手でコミットし直さない。
- 道具（`scripts/`・`Makefile`・`isu.env`・`docs/kit`・`docs/flow`・`.claude/`）を変えたら、その手引きと 1 コミットにし、`make lint` と `make test` を通す。練習用のリポジトリで確かめたら、isucon-kit に移す。

## 困ったら

- 何かおかしい → `make doctor`（ssh・サービス・ディスクなどを診断して次の一手を出す）。
- ssh がつながらない → 人間に伝える（IP 変更なら人間が `isuenv ssh <環境名>` を実行する）。
- 戻したい → `make best` で良かったコミットを探し、`make deploy REV=<rev>`。
- 設定が壊れていた場合、nginx / MySQL は deploy が自動で元に戻している（退避先は `/var/backups/isu-deploy/`）。
- DB を別サーバーに分けるとき：MySQL の `bind-address`、ユーザーの接続元ホスト（`'%'`）、アプリの接続先を private IP に。
