# CLAUDE.md

Webアプリのパフォーマンスチューニング（ISUCON形式）を、人間1人と Claude で行うリポジトリ。

## 前提

- 文書の目次は `README.md`。道具ごとの手引きは `docs/kit/`、場面ごとの手順は `docs/flow/`、docs の規則は `docs/README.md`。
- サーバー構成・アプリの場所・ベンチの方法は `isu.env` に書いてある。道具の既定値は `scripts/lib.sh`。
- **このリポジトリが正**。サーバーへの反映は `git commit` → `make deploy` だけで行う。
- サーバーに置く設定ファイルは `server/all/<絶対パス>`（全台）か `server/<ホスト名>/<絶対パス>`（その台だけ）。既存の設定を変えるときは `make pull F=/etc/...` で取り込んでから編集する。
- DB のスキーマ変更は `sql/NNN_*.sql`（何度流しても壊れない形で）。
- 操作は `make help` を参照。
- 手元だけの環境変数（`AWS_PROFILE` など）は `.envrc`（direnv、git 管理外。雛形は `.envrc.example`）。Claude の Bash には direnv のフックが効かないので、`direnv exec . make ...` で実行する（.claude/settings.json で許可してある）。
- 計測の結果は `logs/`（make analyze）と `pprotein/data/`（make pp-up 中）にある。ベンチの記録は `docs/scores.tsv`。

## 出来事ごとの手順

いつ何を書き、いつ何を起動し、いつコミットし、いつ何を読むかは、ここに従う。判断の入る言葉（区切り・小さな施策など）は使わない。

### 文書が要るかどうか

- アプリ（`APP_LOCAL`）・`server/`・`sql/` を変えるコミットは「施策」とする。design と results の両方を必ず書く。
  - ただし、道具が自分で作るコミット（`chore: 計測を外す`／`chore: 計測を戻す`、`chore: ベンチ結果を記録する`）は施策に数えない。
- `scripts/`・`Makefile`・`isu.env`・`docs/kit`・`docs/flow`・`.claude/` を変えるコミットは「道具の変更」とする。design と results は書かず、その道具の手引きを同じコミットで直す。

### 施策 1 回の流れ

1. セッションを始めたとき：hooks が、ベストスコアの上位と、最新の results の「次の候補」を読み込ませる。
2. 次の施策を決めるとき：最新の results の「次の候補」と、直近の `make analyze` を読んで 1 つ選ぶ。1 施策は 1 コミットにする。
3. コードを書く前：`make snapshot` で基準を取る。doc-writer に design を書かせる（裏で走らせ、完了を待たずに実装を始める）。
4. 施策のコードを書き終えたとき：`perf:`（または `fix:`）で 1 コミットにし、`make deploy` → `make compare` を実行する。compare の違いが意図どおりなら、`make snapshot` で基準を取り直す。
5. `make bench E="期待した動き"` の後（make bench は前面で実行する）：bench が `docs/scores.tsv` を自分でコミットする。`make analyze` を実行し、doc-writer に results を書かせる（番号は design と同じ）。施策を取りやめたときも、results に「取りやめ」として書く。
6. doc-writer がコミットを返したとき：すぐに `make docs-merge` で main に取り込む。
7. 依頼された作業を終えて報告する前（途中で人間に質問するときは含めない）：最後の reboot-test より後に deploy していれば、`make reboot-test` → `make bench` を 1 回実行する。判断には `logs/deployed` と `logs/*-rows-after.tsv` の時刻を使う。hooks が取りこぼし（results の無い design、main に入っていない docs）を確かめる。

### 道具を変えるとき

- 変更と、その道具の手引き（`docs/kit`）の更新を 1 コミットにし、`make lint` と `make test` を通す。
- 練習用のリポジトリで確かめたら、その場で isucon-kit に移す。

### LLM が参照するタイミング

- 毎回：この CLAUDE.md と、hooks が読み込ませる内容（ベストスコア・最新の results の次の候補・docs の状態）。
- ある道具をそのセッションで初めて使う前と、その道具が失敗したとき：その道具の `docs/kit/<道具>.md`。
- 文書を書くとき（doc-writer）：`docs/README.md`。
- 施策を選ぶ前：最新の results と、直近の `make analyze` の結果。

## 自由にやってよいこと

- 施策の内容・順番・使うツールはすべて自由。ツールの追加やスクリプトの改良もしてよい。
- ssh での調査（ログを見る、計測する、一時的に試す）。
- 人間だけのもの（`make up` / `make down`）以外の make の実行。`make deploy` / `make bench` / `make analyze` / `make reboot-test` / `make measure-*` / `make snapshot` / `make compare` / `make patrol` / `make local-*` / `make docs-*` / `make lint` / `make test` など。
- doc-writer の起動と、`make docs-merge` による docs ブランチの取り込み（push はしない）。

## 守ること

1. **最終状態は必ず `make deploy` で再現できること。** サーバー上で直接変えたものは、リポジトリに取り込む（`make pull`）か元に戻す。
2. **deploy の前にコミットする。** ベンチ結果はコミットに紐づけて記録される（`make best` で振り返れる）。
3. **次は触らない**（壊すと外から入れなくなり、復旧できない）：
   - ssh / sshd の設定と停止
   - ネットワーク設定（netplan・IP・ルーティング・MTU・resolv.conf・hosts）
   - ファイアウォール（ufw・iptables・nftables）、AWS（aws CLI、isuenv up/down）
   - 外から見える 80/443 の待ち受けを止めること
   - `git push`
4. **失敗を握りつぶさない。** `|| true` やエラーの無視でビルド・設定チェックを通さない。
5. **レギュレーション**：再起動後も動く／書き込んだデータが再起動後も残る／`/initialize` が動く／ベンチ中の処理を外部に委譲しない。

## コミット

- 施策・機能ごとに 1 コミットにする。複数の施策を混ぜてからまとめてコミットしない（ベンチで効果を切り分けられなくなる）。
- メッセージは `prefix: 何を変えたか` の 1 行で書く。「〜するため」で終わる理由だけの書き方はしない。
  - 例：`perf: 画像を nginx から静的配信する`、`fix: analyze の alp 出力項目を 1.0 系に合わせる`
- prefix は次から選ぶ。
  - `perf`：性能改善（インデックス・クエリ・キャッシュ・設定チューニング）
  - `fix`：不具合の修正
  - `feat`：機能やツールの追加
  - `refactor`：動きを変えない整理
  - `chore`：ベンチ結果の記録・依存や設定の雑務
  - `docs`：ドキュメント
- 誰がいつコミットするか：
  - 施策：コードを書き終えたら、Claude が 1 コミットにする（`perf:` か `fix:`）。
  - 記録：道具がその場でコミットする（make bench・score・portal の `chore: ベンチ結果を記録する`、make measure-off / on の `chore: 計測を外す`／`chore: 計測を戻す`）。手でコミットし直さない。
  - 積み上げる文書：doc-writer が docs ブランチにコミットする（`docs:`）。Claude が直後に `make docs-merge` で main に取り込む。
  - 道具の変更：変更と手引きを 1 コミットにする（`feat:` / `fix:` / `refactor:`）。
- フッター（Co-Authored-By など）は付けない。

## 文書の扱い

main のチェックアウトはチューニング（コミット → deploy → bench）で使っているため、積み上げる文書（overview・design・results・retro）は docs ブランチで書く。

- 積み上げる文書は、サブエージェント `doc-writer` に書かせる。docs ブランチの worktree（`.claude/worktrees/docs`）に書いてコミットする。規則は `docs/README.md`。
- doc-writer がコミットを返したら、Claude がすぐに `make docs-merge` で main に早送りで取り込む。GitHub への push は人間が行う。GitHub の main をローカルの main からしか進めないことで、pull と、ベンチ記録の SHA を変える rebase を避ける。
- 生きた文書（README・CLAUDE.md・docs/README・docs/kit・docs/flow）は、道具や手順を変えたコミットで、変えたセッションが main に直接書く。
- worktree からは `make deploy` / `make bench` を実行しない（deploy はそのチェックアウトの HEAD をサーバーに反映するため）。
- 施策を進める（bench を回し、doc-writer を起動する）セッションは 1 つにする。hooks の取りこぼしの確認はリポジトリ全体の状態を見るので、ほかのセッションにも同じ案内が出ることがある。施策を進めていないセッションでは、results の案内に従って doc-writer を起動しない（`make docs-merge` の案内には従ってよい）。
- `CLAUDE.md` の変更は、main に入るまで各セッションに効かない。`.claude/settings.json` の hooks は、通常はセッションの途中でも読み直される。

## 困ったら

- 何かおかしい → `make doctor`（ssh・サービス・ディスクなどを診断して次の一手を出す）。
- ssh がつながらない → 人間に伝える（IP 変更なら人間が `isuenv ssh <環境名>` を実行する）。
- 戻したい → `make best` で良かったコミットを探し、`make deploy REV=<rev>`。
- 設定が壊れていた場合、nginx / MySQL は deploy が自動で元に戻している（退避先は `/var/backups/isu-deploy/`）。
- DB を別サーバーに分けるとき：MySQL の `bind-address`、ユーザーの接続元ホスト（`'%'`）、アプリの接続先を private IP に。
