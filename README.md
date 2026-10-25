# isucon-kit

1人 + Claude Code で ISUCON 形式のチューニングをするための土台。
施策・順番・ツールは自由。キットが受け持つのは「壊さない・戻せる・つながる」ことだけ。

```
isu.env        問題ごとに変えるのはここだけ（サーバー、アプリの場所、ビルド、ベンチ）
CLAUDE.md      Claude へのルール（最小限）
Makefile       操作の入口（make help）
server/all/    全台に置く設定ファイル（絶対パスのミラー：server/all/etc/nginx/... → /etc/nginx/...）
server/<host>/ その台だけに置く設定ファイル
sql/           DB のスキーマ変更（deploy のたびに適用）
notes/         scores.tsv（自動記録）、memo.md（自由）
```

## 最初の1回：リポジトリを作ってファイルを取り込む

キットがリポジトリの土台になる。ISUCON のアプリは**サーバー上で動いているもの**を取り込んで、同じリポジトリに入れる（公式リポジトリを clone する必要はない）。
アプリ・設定・SQL が1つのコミットにそろうので、`make deploy REV=<rev>` で丸ごと戻せる。

```bash
# 1. キットから新しいリポジトリを作る（問題ごとに1つ）
unzip isucon-kit.zip && mv isucon-kit practice-private-isu && cd practice-private-isu
git init && git add -A && git commit -m "kit"

# 2. isu.env を問題に合わせる（private-isu はそのままでよい）

# 3. 環境を作り、計測ツールを入れる
make up                       # isuenv で競技1台 + ベンチ1台（TTL 4h）
make setup                    # 計測ツール導入、Ruby→Go 切替（isu.env の SETUP_CMD）

# 4. アプリを取り込む（APP_REMOTE → APP_LOCAL。APP_EXCLUDE のビルド成果物は除外）
make pull-app
git status                    # 余計なもの（バイナリ・巨大ファイル）が入っていないか確認
git add -A && git commit -m "baseline: app"

# 5. 設定ファイルを取り込む（いま全部やらなくてよい。変えたくなったときに都度）
make pull F=/etc/nginx/nginx.conf
make pull F=/etc/nginx/sites-available/isucon.conf
git add -A && git commit -m "baseline: nginx"

# 6. 何も変えていない状態で一周させる（ここで初期スコアを記録）
make deploy && make bench && make analyze
```

ここまで通ったら、Claude Code Desktop でこのフォルダを開いて好きに進める。

取り込むときの注意：

- **取り込んだら、編集する前にそのままコミットする。** baseline と比べて、何を変えたかが差分で見えるようにするため。元の内容のまま deploy しても何も変わらないので、6 の一周は「キットが正しく動くか」の確認にもなる。
- **取り込むのは実体のファイル。** `sites-enabled/` のようなシンボリックリンクではなく、リンク先の `sites-available/` 側を取り込む（deploy はリンクを配らない）。
- **入れないもの**：ビルド成果物、`vendor/` などの巨大ファイル、DB ダンプや画像データ、パスワードや鍵。`make pull-app` で入ってしまったら、`isu.env` の `APP_EXCLUDE` か `.gitignore` に足して取り除く。
- **ホストごとに違う設定**は `make pull F=... H=<ホスト名>` で `server/<ホスト名>/` に取り込む（複数台構成のとき）。
- `/home/isucon/...` 配下のファイル（`env.sh` など）も同じように取り込める。deploy では所有者（`isucon`）を保ったまま書き戻される。
- バックアップが欲しければ、GitHub の private リポジトリを作って人間が push する（`git remote add origin ...` → `git push -u origin main`）。Claude には push を禁止している。

## 2回目以降（環境は TTL で消えている）

```bash
make up && make setup && make deploy    # 前回の最終コミットの状態に戻る
```

## 日々の流れ

- 変更 → `git commit` → `make deploy` → `make bench` → `make analyze`
- 良かった版に戻す：`make best` → `make deploy REV=<rev>`
- おかしい：`make doctor`

## トラブルに強くしてある点

| 起きがちなこと | キットの対応 |
|---|---|
| 未コミットの変更をデプロイして、何をベンチしたか分からなくなる | deploy はコミットしか配らない（`git archive`）。スコアはコミットと一緒に記録 |
| nginx / MySQL の設定ミスで起動しなくなる | `nginx -t` 失敗・MySQL 起動失敗を検知して、自動で元の設定に戻す |
| 置き換えた元の設定が分からなくなる | `/var/backups/isu-deploy/<時刻>-<rev>/` に退避 |
| ssh・ネットワーク・FW をいじって入れなくなる | 該当ファイルは deploy が拒否。Claude の設定でも禁止 |
| ビルド失敗に気づかず古いバイナリでベンチ | ビルド・疎通確認の失敗で deploy が止まる |
| deploy とベンチが重なる | ロックで直列化 |
| 回線断で ssh が固まる・遅い | タイムアウト、keepalive、接続の使い回しを設定済み |
| 自宅↔外で IP が変わってつながらない | `make doctor` が原因を判定して対処を表示（人間が `isuenv ssh <環境名>`） |
| TTL 切れで環境が消えた | `make doctor` が検出。`make up && make setup && make deploy` で復元 |
| 再起動で動かなくなる（追試で失格） | `make reboot-test` |

## 本番で使うとき

「最初の1回」と同じ流れで、開始直後に新しいリポジトリを作る。違うのは 2 と 3 だけ：`make up` は使わず、ポータルの3台を `~/.ssh/config` に書いてから `make setup`。

`isu.env` を書き換える：`ENV=""`、`HOSTS` を `~/.ssh/config` に書いた3台の名前に、アプリの場所・サービス名・DB 接続、`BENCH_CMD=""`（ベンチはポータルから）。
ベンチは「`make prebench` → ポータルで実行 → `make analyze` → `make score S=<点数>`」。

最終盤は計測を外す：`server/all/etc/nginx/conf.d/isu-log.conf` を消し、`isu-slow.cnf` を `slow_query_log = 0` にして commit → deploy → `make reboot-test` → ベンチで確認。

## 初回に合わなければ直すところ

実サーバーでは未検証の前提がある。合わなければ Claude に「make setup が失敗した。原因を調べて直して」と頼めばよい。

- ssh ユーザーが sudo でき、アプリは `APP_USER` で動く
- `BUILD_CMD` の go が `APP_USER` のログインシェルで見つかる
- nginx の `conf.d/*.conf` が http ブロックで読み込まれ、server 側に `access_log` が無い
- MySQL が `/etc/mysql/conf.d/` を読み込む
- Claude の権限は `.claude/settings.json`（ssh・make・git は許可、環境操作と push は禁止）。もっと慎重にしたければ `ssh` を `ask` に移す
