# setup

新しいサーバーの準備と、サーバー上のアプリ・設定ファイルの取り込みを受け持ちます。

## 役割

- `make setup` は、計測ツール（alp・pt-query-digest・htop・dstat・sysstat）と pprotein-agent を入れ、isu.env の `SETUP_CMD` を実行する。何度実行しても同じ結果になる。
- `make setup` は、apt の自動更新（apt-daily・unattended-upgrades）と snapd を止める。ベンチ中に走ると CPU とディスクを取られ、スコアがぶれるため。実行中の apt は止めず、ロックが空くのを待つ。
- `make pull-app` は、サーバーのアプリ（`APP_REMOTE`）を `APP_LOCAL` に取り込む。最初の 1 回だけ使う。
- `make pull F=<絶対パス> [H=<ホスト>]` は、サーバーの設定ファイルを server/all/（全台）か server/<ホスト>/ に取り込む。
- 設定ファイルの反映は受け持たない（docs/kit/deploy.md）。
- worktree（調査役の下ごしらえの場所）では断る（docs/kit/deploy.md）。

## いつ使うか

- 新しいサーバーで最初に 1 回：`make setup` → `make pull-app`（docs/flow/first-time.md）。TTL 切れで作り直したときも `make setup`（docs/flow/resume.md）。
- サーバーの既存の設定を変える前：`make pull F=...` で取り込んでから編集し、commit → `make deploy`。

## 動かし方

- scripts/setup.sh と scripts/pull.sh の冒頭のコメント、`make help`。

## 確かめ方

- setup の最後に、各ツールのパスと、pprotein-agent が active であることが表示される。
- 取り込んだら `git status` で、余計なもの（ビルド成果物・巨大ファイル）が入っていないかを見る。入っていたら isu.env の `APP_EXCLUDE` か .gitignore に足す。
- sites-enabled/ のようなシンボリックリンクではなく、リンク先（sites-available/）を取り込む。deploy はリンクを配らない。

## 更新のしかた

- 入れるツール：scripts/setup.sh の `need+=` と、最後の表示の `for t in ...`。
- 問題ごとの追加コマンド：isu.env の `SETUP_CMD`（冪等に書く）。
- 直したら `make lint`。練習環境で `make setup` を 2 回流し、同じ結果になることを見る。
