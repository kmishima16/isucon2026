# analyze

直前のベンチの計測を集計し、次の施策の手がかりを出します。

## 役割

- `make analyze` は、alp（アクセスログ）・pt-query-digest（スローログ）・pprof・fgprof と、各台の vmstat・pidstat・memcached の stats をまとめて出し、logs/<時刻>-* に保存する。
- vmstat・pidstat・memcached は、prebench が各台で始め（scripts/sysstat.sh start）、analyze が alp と pt-query-digest より先に回収する（scripts/sysstat.sh collect）。後に回すと、集計の CPU と、ベンチの終わりから analyze までの空きが平均に入る。台の上で取るので、ポータルのベンチでも同じ手順で使える。窓はベンチより長く取り（ポータルのベンチではさらに長く）、集計はベンチ中の秒だけを使う（vmstat は前後のアイドルを除き、pidstat は CPU を使っていた区間だけで平均する）。
- `make prof` は、ベンチ中に pprof と fgprof を取る。`make bench PROFILE=1` でも自動で取る。手元に go が無ければ、サーバーの go で集計する。
- どの施策をするかは決めない。操作役がここから候補を出し、人間が選ぶ（.claude/skills/isu-ops/SKILL.md の 3〜4）。

## いつ使うか

- 毎回の bench の後（.claude/skills/isu-ops/SKILL.md の 6）。施策の結果を確かめる元になる。
- 次の候補を出す前（同じスキルの 3）。

## 動かし方

- scripts/analyze.sh・scripts/sysstat.sh・scripts/prof.sh の冒頭のコメント、`make help`。

## 確かめ方

- alp の表・pt-query-digest の Profile・「[台] リソース」の 3 つが出る。`SLOW_LOG_KIND="pg"` では、pt-query-digest の代わりに slp pg の表（合計時間の降順、tsv）が `logs/<時刻>-slow.txt` に出る。ログを手元に持ってきて、`.tools/slp-bin`（make pp-up が入れる）で集計する。
- 「[台] リソース」の vmstat の行は、id が 90% 未満の最初の秒から最後の秒までの平均で、間のアイドルも含む。走行前の整合性チェックが長い問題では、初期化から走行の始まりまでの空きが入り、CPU が空いて見える。CPU の空きは、`logs/<時刻>-<台>/vmstat.txt` の負荷の区間の毎秒の行で見る。
- 「/tmp/isu-measure が無い」と出たら、prebench の後にサーバーを再起動している。もう一度 bench する。
- pprof が取れないときは、`make measure-show` でアプリが pprof を待ち受けているかを見る（docs/kit/measure.md）。

## 更新のしかた

- alp の URI のまとめ方：isu.env の `ALP_MATCH`。
- 計測の秒数：scripts/lib.sh の `MEASURE_SECONDS`（手元でベンチ）と `PORTAL_MEASURE_SECONDS`（BENCH_CMD が空でポータルからベンチ）。変えるときは isu.env に書く。走行前のチェックが長い問題では、チェックと走行を足した時間より長くする（ISUCON13 では `MEASURE_SECONDS` と `PPROTEIN_SECONDS` を 150 にした）。直したら `make test`（tests/test_sysstat.sh）。
- pprof の URL：isu.env の `PPROF_URL`・`FGPROF_URL`。DB 待ちも見たいなら、アプリに fgprof（github.com/felixge/fgprof）を /debug/fgprof で足し、`FGPROF_URL` と `PPROTEIN_PPROF_PATHS` に加える。
- 直したら `make lint`。練習環境で bench → analyze を通す。
