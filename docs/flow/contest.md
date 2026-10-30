# 本番

ポータルの 3 台で始める手順と、最終盤（計測を外す → 再起動試験 → ベンチ）の手順です。

## 始め方

- 最初の 1 回（docs/flow/first-time.md）と同じ流れで、開始直後に新しいリポジトリを作る。違うのは手順 2 と 3 だけにする。make up は使わず、ポータルの 3 台を ~/.ssh/config に書いてから `make setup`。
- isu.env を書き換える：`ENV=""`、`HOSTS` を ~/.ssh/config に書いた 3 台の名前に、アプリの場所・サービス名・DB の接続、`BENCH_CMD=""`（ベンチはポータルから）、`PORTAL_RESULT_URL`（docs/kit/portal.md）。
- DB を別の台に分けるとき：MySQL の bind-address、ユーザーの接続元ホスト（'%'）、アプリの接続先を private IP にする。

## ベンチ

- `make prebench` → ポータルでベンチを実行 → `make portal WATCH=1` → `make analyze` の順にする。取り出せないときは `make score S=<点数>`。
- 施策 1 回の流れは、CLAUDE.md の出来事ごとの手順のままにする。

## 最終盤

1. `make measure-off`：アクセスログ・スローログ・pprof を外して commit → deploy する（docs/kit/measure.md）。
2. `make pp-agent-off`：各台の pprotein-agent を止める（再起動しても起きない）。
3. `make reboot-test`：行数の比較と、DB があとから起きる場合を確かめる（docs/kit/reboot-test.md）。
4. ポータルでベンチを実行し、スコア帯を確かめる → `make portal`。
