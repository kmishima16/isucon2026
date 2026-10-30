# 本番

ポータルの 3 台で始める手順と、最終盤（計測を外す → 再起動試験 → ベンチ）の手順です。

## 始め方

- 最初の 1 回（docs/flow/first-time.md）と同じ流れで、開始直後に新しいリポジトリを作る。違うのは手順 2 と 3 だけにする。make up は使わず、ポータルの 3 台を ~/.ssh/config に書いてから `make setup`。
- isu.env を書き換える：`ENV=""`、`HOSTS` を ~/.ssh/config に書いた 3 台の名前に、アプリの場所・サービス名・DB の接続、`BENCH_CMD=""`（ベンチはポータルから）、`PORTAL_RESULT_URL`（docs/kit/portal.md）。
- DB を別の台に分けるとき：MySQL の bind-address、ユーザーの接続元ホスト（'%'）、アプリの接続先を private IP にする。
- GitHub に private リポジトリを作り（人間）、操作役のセッションで `/isu-ops`、調査役のセッションで `/isu-research` を打つ。

## 時間割（10:00〜18:00）

- 10:00：人間がマニュアルを読み、サーバーを立てて ~/.ssh/config に書く。
- 10:10：操作役が isu.env を直し、最初の一周（setup → pull-app → コミット → deploy → 初期ベンチ）を回す。
- 10:30：調査役と人間が並行して、採点の仕組み・コード・業務ロジックを読む。操作役は計測から候補を出す。
- 11:30：人間が大きな方針（メモリに持つか、台の分け方）を決める。以後、施策を回す（.claude/skills/isu-ops/SKILL.md）。
- 14:00：通し稽古（計測を外す → 全台を再起動 → ベンチ → 戻す）。
- 15:00：大きな方針が pass していなければ、`make best` の版に戻す。
- 15:30：大きな変更を止める。
- 16:30：最終盤（下）。再起動は順序を変えて 2 通り。
- 17:30 以降：変えない。18:00 以降も、追試が終わるまで EC2 を止めない。

## ベンチ

- `make prebench` → ポータルでベンチを実行 → `make portal WATCH=1` → `make analyze` の順にする。取り出せないときは `make score S=<点数>`。
- 施策 1 回の流れは、操作役のスキル（.claude/skills/isu-ops/SKILL.md）に従う。

## 最終盤

1. `make measure-off`：アクセスログ・スローログ・pprof を外して commit → deploy する（docs/kit/measure.md）。
2. `make pp-agent-off`：各台の pprotein-agent を止める（再起動しても起きない）。
3. `make reboot-test`：行数の比較と、DB があとから起きる場合を確かめる（docs/kit/reboot-test.md）。
4. ポータルでベンチを実行し、スコア帯を確かめる → `make portal`。
