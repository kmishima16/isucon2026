# bench

ベンチの実行と、結果の記録（docs/scores.tsv）を受け持ちます。

## 役割

- `make prebench` は、計測ログ（アクセスログ・スローログ）を空にし（`truncate -c`。PostgreSQL のログの置き場所では、作る指定で開くと root でも断られる）、vmstat・pidstat・memcached の計測（docs/kit/analyze.md）と pprotein の収集を始める。
- `make bench` は、prebench → isu.env の `BENCH_CMD` でベンチ → 結果を docs/scores.tsv に記録する。`E="期待した動き"` で施策の狙いを一緒に残す。
- `make score S=<点数> [F=<fail の件数>]` は、ポータルのベンチなどを手で記録する。
- `make best` は、記録の上位を出す（良かったコミットを探す）。pass が false の行と fail のある行は出さない。pass でも fail が出ていることがあり、その版に戻すのを防ぐため。pass を読めなかった行（null など）は残す。fail は数でなければ空で記録する。fail が分からない古い記録は `fail=?` で出す。
- 記録したら、docs/scores.tsv だけをその場でコミットする（`chore: ベンチ結果を記録する`）。未コミットのまま次の deploy が止まるのを防ぐため。ほかの変更は巻き込まない。ほかの git の操作（index.lock）と重なったら、外れるまで最長 30 秒待ってやり直す。
- HEAD とデプロイ済みで docs/ 以外が違えば、警告する。
- worktree（調査役の下ごしらえの場所）では断る（docs/kit/deploy.md）。

## いつ使うか

- deploy した後（.claude/skills/isu-ops/SKILL.md の 6）。BENCH_CMD でベンチするときは、裏で回してよい。deploy はロックで止まるので重ならない。ポータルのベンチにはロックがかからないので、終わるまで deploy しない。
- 本番：`make prebench` → ポータルでベンチ → 人間が見たスコアを `make score S=<点数> [F=<fail の件数>]` で記録する（fail していれば `PASS=false` も）。`make portal` は、取り出せると分かったときだけ使う（docs/kit/portal.md）。

## 動かし方

- scripts/bench.sh の冒頭のコメント、`make help`。`PROFILE=1` で pprof・fgprof も取る（docs/kit/analyze.md）。

## 確かめ方

- 最後に「記録: rev=... pass=... score=...」が出る。`git log -1` が `chore: ベンチ結果を記録する` になっている。
- スコアを読み取れなかったときは、logs/<時刻>-bench.txt を見て `make score S=<点数>`。
- docs/scores.tsv の列は time・rev・pass・score・memo・expect・fail。fail は結果の JSON の `fail`（make bench）、`PORTAL_JQ_FAIL`（make portal）、`F=`（make score）から入る。

## 更新のしかた

- ベンチのコマンド：isu.env の `BENCH_CMD`（本番では空にする）。
- 結果の読み取り：scripts/bench.sh の `RESULT`・`PASS`・`SCORE`（`{"pass":...,"score":...}` の形を前提にしている）。
- 直したら `make lint` と `make test`（tests/test_bench_record.sh）。
