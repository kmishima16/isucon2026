# measure

計測（nginx のアクセスログ・DB のログと集計・アプリの pprof）を、まとめて外す・戻す・確かめます。DB は MySQL のスローログか、PostgreSQL の全クエリのログと pg_stat_statements です。

## 役割

- `make measure-off` / `make measure-on` は、server/ の設定ファイルを書き換えて commit（`chore: 計測を外す` / `chore: 計測を戻す`）→ deploy までを行う。
- pprof は、アプリが環境変数 `ISU_MEASURE=0` のときに待ち受けないように作っておく。値は systemd の drop-in で渡す。
- DB は、リポジトリにある方の設定だけを書き換える。
  - MySQL：`server/all/etc/mysql/conf.d/isu-slow.cnf` の `slow_query_log`。
  - PostgreSQL：`server/<台>/etc/postgresql/<版>/main/conf.d/isu-measure.conf` の `log_min_duration_statement`（0 / -1）・`pg_stat_statements.track`（all / none）・`track_io_timing`（on / off）。3 つとも reload で効き、deploy が reload する。`pg_stat_statements` の読み込み（`shared_preload_libraries`）は restart が要るので、外さずに残す。
- 設定ファイルは消さずに、中身の値で外す。deploy は、リポジトリで消したファイルをサーバーから消さないため。
- `make measure-show` は、リポジトリの設定と、各台の実際の状態を並べて出す。DB の状態は、isu.env の `SLOW_LOG_KIND`（my / pg）の種類で問い合わせる。
- 受け持つのは nginx・MySQL・PostgreSQL・systemd の構成だけ。
- worktree（調査役の下ごしらえの場所）では断る（docs/kit/deploy.md）。

## いつ使うか

- 本番の最終盤：`make measure-off`（docs/flow/contest.md）。
- 計測の負荷を見積もりたいとき：外してベンチし、戻す。
- 計測が効いているか分からないとき：`make measure-show`。

## 動かし方

- scripts/measure.sh の冒頭のコメント、`make help`。

## 確かめ方

- `make measure-show` で、リポジトリの設定（nginx=1 mysql=- pg=1 app=1 など。- はその DB の設定がリポジトリに無いこと）と、各台の状態（access_log・slow_query_log か log_min_duration_statement・pprof の応答）が一致する。
- 外したあと、アクセスログと DB のログの大きさが増えない。

## 更新のしかた

- 書き換える設定ファイル：scripts/measure.sh の `NGINX_CONF`・`MYSQL_CONF`・`PG_CONFS`（server/ の下から探す）・`DROPIN`。
- アプリ側の受け口：アプリの main で `ISU_MEASURE` を見る（Go では net/http/pprof と fgprof を localhost:6060 で待ち受ける）。
- PostgreSQL の設定の反映と確かめ：scripts/apply-overlay.sh（docs/kit/deploy.md）。
- 直したら `make lint` と `make test`。練習環境で off → show → on → show を通す。
