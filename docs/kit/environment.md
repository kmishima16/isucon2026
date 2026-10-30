# environment

練習環境（isuenv）の作成・削除・一覧と、手元だけの環境変数（.envrc）を受け持ちます。

## 役割

- `make up` / `make down` で、isuenv の練習環境（競技 1 台とベンチ 1 台）を作る・消す。AWS を操作するので人間だけが実行する（Claude には .claude/settings.json で禁止している）。
- `make list` で、稼働中の環境・残りの TTL・概算のコストを見る。
- 手元だけの環境変数（`AWS_PROFILE`、本番のポータルの Cookie）は `.envrc` に書き、direnv で読み込む。`.envrc` は git に入れない。
- 本番の 3 台は受け持たない（docs/flow/contest.md）。

## いつ使うか

- 練習を始めるとき・TTL が切れたとき：人間が `make up`（docs/flow/first-time.md・docs/flow/resume.md）。
- 練習を終えるとき：人間が `make down`。
- 残り時間を知りたいとき：`make list`。

## 動かし方

- `make help` と Makefile の `up`・`down`・`list` を見る。`.envrc` の雛形は `.envrc.example`。
- Claude の Bash には direnv のフックが効かないので、Claude は `direnv exec . make ...` で実行する。.claude/settings.json はこの形を許可し、`direnv exec . make up` / `down` は禁止している。

## 確かめ方

- `direnv exec . make list` で、自分の環境が表示される。
- 「login session has expired」と出たら、人間が `aws login` し直す。`.envrc` の `AWS_PROFILE` がそのプロファイルを指しているかも見る。
- 「.envrc is blocked」と出たら、人間が `direnv allow` を実行する。

## 更新のしかた

- 環境名：isu.env の `ENV`。TTL は `make up TTL=6h` のように渡す（既定の 4h は Makefile の `up`）。
- 手元の環境変数を足す：`.envrc.example` に書き、各自が `.envrc` に写す。
- Claude の許可と禁止：.claude/settings.json。`make` の許可や禁止を足したら、`direnv exec . make` の形も揃える。
- 直したら `make lint`。
