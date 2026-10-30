# snapshot

主なページの応答を取っておき、施策の後に同じ状態で取り直して比べる、回帰チェックです。

## 役割

- `make snapshot [NAME=base]` は、`SNAPSHOT_INIT`（/initialize）を叩いてデータをそろえ、`SNAPSHOT_URLS` の応答（本文・ステータス・Content-Type）を logs/snapshot/<NAME>/ に保存する。
- `make compare [BASE=base] [NAME=current]` は、同じ状態で取り直して比べ、違えば差分を出して失敗する。毎回変わる部分（CSRF トークンなど）は `SNAPSHOT_IGNORE` で消してから比べる。
- 比べるのは、未ログインの GET だけ。ログイン後の画面と操作は docs/kit/patrol.md。
- ベンチでは表示や応答の変化に気づきにくいので、それを補う。

## いつ使うか

- 施策のコードを書く前：`make snapshot`（CLAUDE.md の出来事ごとの手順 3）。
- deploy の直後：`make compare`（手順 4）。違いが意図どおりなら、`make snapshot` で取り直す。

## 動かし方

- scripts/snapshot.sh の冒頭のコメント、`make help`。

## 確かめ方

- compare の最後に「すべて同じでした」が出る。違いがあれば、ページごとに DIFF と差分の先頭が出る。
- /initialize が遅い・失敗するときは、`SNAPSHOT_INIT` の応答を確かめる（60 秒で打ち切る）。

## 更新のしかた

- 比べるページと、消す部分：isu.env の `SNAPSHOT_URLS`・`SNAPSHOT_IGNORE`（問題ごとに書き直す）。
- 直したら `make lint`。練習環境で snapshot → compare が「すべて同じ」になり、わざと変えると DIFF になることを見る。
