# analyze

直前のベンチの計測を集計し、次の施策の手がかりを出します。

## 役割

- `make analyze` は、alp（アクセスログ）・pt-query-digest（スローログ）・pprof・fgprof と、各台の vmstat・pidstat・memcached の stats をまとめて出し、logs/<時刻>-* に保存する。
- vmstat・pidstat・memcached は、prebench が各台で始め（scripts/sysstat.sh start）、analyze が回収する（scripts/sysstat.sh collect）。台の上で取るので、ポータルのベンチでも同じ手順で使える。
- `make prof` は、ベンチ中に pprof と fgprof を取る。`make bench PROFILE=1` でも自動で取る。手元に go が無ければ、サーバーの go で集計する。
- どの施策をするかは決めない。それは最新の results と合わせて決める（CLAUDE.md の出来事ごとの手順 2）。

## いつ使うか

- 毎回の bench の後（手順 5）。results の「計測の変化」の元になる。
- 次の施策を決める前（手順 2）。

## 動かし方

- scripts/analyze.sh・scripts/sysstat.sh・scripts/prof.sh の冒頭のコメント、`make help`。

## 確かめ方

- alp の表・pt-query-digest の Profile・「[台] リソース」の 3 つが出る。
- 「/tmp/isu-measure が無い」と出たら、prebench の後にサーバーを再起動している。もう一度 bench する。
- pprof が取れないときは、`make measure-show` でアプリが pprof を待ち受けているかを見る（docs/kit/measure.md）。

## 更新のしかた

- alp の URI のまとめ方：isu.env の `ALP_MATCH`。
- 計測の秒数：scripts/lib.sh の `MEASURE_SECONDS`（変えるときは isu.env に書く）。
- pprof の URL：isu.env の `PPROF_URL`・`FGPROF_URL`。DB 待ちも見たいなら、アプリに fgprof（github.com/felixge/fgprof）を /debug/fgprof で足し、`FGPROF_URL` と `PPROTEIN_PPROF_PATHS` に加える。
- 直したら `make lint`。練習環境で bench → analyze を通す。
