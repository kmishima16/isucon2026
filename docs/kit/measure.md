# measure

計測（nginx のアクセスログ・MySQL のスローログ・アプリの pprof）を、まとめて外す・戻す・確かめます。

## 役割

- `make measure-off` / `make measure-on` は、server/ の設定ファイルを書き換えて commit（`chore: 計測を外す` / `chore: 計測を戻す`）→ deploy までを行う。
- pprof は、アプリが環境変数 `ISU_MEASURE=0` のときに待ち受けないように作っておく。値は systemd の drop-in で渡す。
- `make measure-show` は、リポジトリの設定と、各台の実際の状態を並べて出す。
- 受け持つのは nginx・MySQL・systemd の構成だけ。

## いつ使うか

- 本番の最終盤：`make measure-off`（docs/flow/contest.md）。
- 計測の負荷を見積もりたいとき：外してベンチし、戻す。
- 計測が効いているか分からないとき：`make measure-show`。

## 動かし方

- scripts/measure.sh の冒頭のコメント、`make help`。

## 確かめ方

- `make measure-show` で、リポジトリの設定（nginx=1 mysql=1 app=1）と、各台の状態（access_log・slow_query_log・pprof の応答）が一致する。
- 外したあと、アクセスログとスローログの大きさが増えない。

## 更新のしかた

- 書き換える設定ファイル：scripts/measure.sh の `NGINX_CONF`・`MYSQL_CONF`・`DROPIN`。
- アプリ側の受け口：アプリの main で `ISU_MEASURE` を見る（Go では net/http/pprof と fgprof を localhost:6060 で待ち受ける）。
- 直したら `make lint`。練習環境で off → show → on → show を通す。
