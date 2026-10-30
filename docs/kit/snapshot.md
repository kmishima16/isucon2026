# snapshot

主なページの応答を取っておき、施策の後に同じ状態で取り直して比べる、回帰チェックです。

## 役割

- `make snapshot [NAME=base]` は、`SNAPSHOT_INIT`（/initialize）を叩いてデータをそろえ、`SNAPSHOT_URLS` の応答（本文・ステータス・Content-Type）を logs/snapshot/<NAME>/ に保存する。
- `make compare [BASE=base] [NAME=current]` は、同じ状態で取り直して比べ、違えば差分を出して失敗する。毎回変わる部分（CSRF トークンなど）は `SNAPSHOT_IGNORE` で消してから比べる。
- 比べるのは、未ログインの GET だけ。ログイン後の画面と操作は docs/kit/patrol.md。
- ベンチでは表示や応答の変化に気づきにくいので、それを補う。
- worktree（調査役の下ごしらえの場所）では断る（docs/kit/deploy.md）。

## いつ使うか

- 基準を取るとき：最初の一周の後に `make snapshot`。snapshot と compare は初期化（`SNAPSHOT_INIT`）を叩くので、ベンチ中に動かすとベンチが fail する。本番ではポータルのベンチが終わってから動かす。
- 応答を作る処理（クエリ・テンプレート・キャッシュ）を変えた施策の deploy の直後：`make compare`（.claude/skills/isu-ops/SKILL.md の 6）。nginx や MySQL の設定だけの施策では行わない。違いが意図どおりなら、`make snapshot` で取り直す。

## 動かし方

- scripts/snapshot.sh の冒頭のコメント、`make help`。

## 確かめ方

- compare の最後に「すべて同じでした」が出る。違いがあれば、ページごとに DIFF と差分の先頭が出る。
- /initialize が遅い・失敗するときは、`SNAPSHOT_INIT` の応答を確かめる（60 秒で打ち切る）。

## 更新のしかた

- 比べるページと、消す部分：isu.env の `SNAPSHOT_URLS`・`SNAPSHOT_IGNORE`（問題ごとに書き直す）。
- 初期化と接続のしかた：`SNAPSHOT_INIT`・`SNAPSHOT_INIT_METHOD`（ISUCON13 は `/api/initialize` を POST）・`SNAPSHOT_BASE`（HTTPS なら https://...）・`SNAPSHOT_CURL_OPTS`（Host ヘッダーや `-k` など。既定値は scripts/lib.sh）。
- 直したら `make lint` と `make test`（tests/test_snapshot.sh）。練習環境で snapshot → compare が「すべて同じ」になり、わざと変えると DIFF になることを見る。
