# CLAUDE.md

Webアプリのパフォーマンスチューニング（ISUCON形式）を、人間1人と Claude で行うリポジトリ。

## 前提

- サーバー構成・アプリの場所・ベンチの方法は `isu.env` に書いてある。
- **このリポジトリが正**。サーバーへの反映は `git commit` → `make deploy` だけで行う。
- サーバーに置く設定ファイルは `server/all/<絶対パス>`（全台）か `server/<ホスト名>/<絶対パス>`（その台だけ）。既存の設定を変えるときは `make pull F=/etc/...` で取り込んでから編集する。
- DB のスキーマ変更は `sql/NNN_*.sql`（何度流しても壊れない形で）。
- 操作は `make help` を参照。
- 手元だけの環境変数（`AWS_PROFILE` など）は `.envrc`（direnv、git 管理外。雛形は `.envrc.example`）。Claude の Bash には direnv のフックが効かないので、`direnv exec . make ...` で実行する。
- 計測の結果は `logs/`（make analyze）と `pprotein/data/`（make pp-up 中。画面は http://localhost:9000、API は `/api/httplog`・`/api/slowlog`・`/api/pprof`）にある。

## 自由にやってよいこと

- 施策の内容・順番・使うツールはすべて自由。ツールの追加やスクリプトの改良もしてよい。
- ssh での調査（ログを見る、計測する、一時的に試す）。
- `make deploy` / `make bench` / `make analyze` / `make reboot-test` の実行。

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
- ベンチ結果（`notes/scores.tsv`）の追記は `chore: ベンチ結果を記録する` で別コミットにする。
- フッター（Co-Authored-By など）は付けない。

## 困ったら

- 何かおかしい → `make doctor`（ssh・サービス・ディスクなどを診断して次の一手を出す）。
- ssh がつながらない → 人間に伝える（IP 変更なら人間が `isuenv ssh <環境名>` を実行する）。
- 戻したい → `make best` で良かったコミットを探し、`make deploy REV=<rev>`。
- 設定が壊れていた場合、nginx / MySQL は deploy が自動で元に戻している（退避先は `/var/backups/isu-deploy/`）。
- DB を別サーバーに分けるとき：MySQL の `bind-address`、ユーザーの接続元ホスト（`'%'`）、アプリの接続先を private IP に。
