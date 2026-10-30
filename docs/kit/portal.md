# portal

本番のポータルのベンチ結果を取り出して表示し、まだ記録していなければ docs/scores.tsv に記録します（骨組み）。

## 役割

- 取り出し方は、isu.env の `PORTAL_RESULT_URL`（Cookie 付きで GET）か `PORTAL_RESULT_CMD`（JSON を出すコマンド）。練習では、直近の make bench の結果を代わりに使う。
- 記録は make score と同じく、その場でコミットする（docs/kit/bench.md）。
- 同じ結果は二重に記録しない。ID か中身で見分け、logs/portal-seen に残す。
- 本番のポータルの形式では、まだ確かめていない。
- worktree（調査役の下ごしらえの場所）では断る（docs/kit/deploy.md）。

## いつ使うか

- 本番：スコアの記録は、人間がポータルで見たスコアを `make score S=<点数>` で記録するのを既定にする。ポータルの結果の形式は当日まで分からず、取り出せないと確かめと切り替えに時間を取られるため。
- ポータルから取り出せると分かったら：ポータルでベンチを始めたら `make portal WATCH=1`（新しい結果が出るまで 15 秒ごとに見に行く。最長 10 分）。その後 `make analyze`。

## 動かし方

- scripts/portal.sh の冒頭のコメント、`make help`。

## 確かめ方

- 「ポータルの結果 <id>: pass=... score=...」と記録が出る。もう一度実行すると「新しい結果はありません」。
- 記録のコミットに失敗したら止まり、その結果を記録済みにしない。原因を直してから、もう一度 `make portal`。

## 更新のしかた

- 使うと決めたら（本番の既定は `make score`）：ポータルを開発者ツールで見て、isu.env の `PORTAL_RESULT_URL` を書く。JSON の形が既定（.pass / .score / .fail / .messages、ID なし）と違えば `PORTAL_JQ_PICK`・`PORTAL_JQ_ID`・`PORTAL_JQ_PASS`・`PORTAL_JQ_SCORE`・`PORTAL_JQ_FAIL`・`PORTAL_JQ_MESSAGES` も書く（既定値は scripts/lib.sh）。
- Cookie は、人間が .envrc の `PORTAL_COOKIE` に写す。Claude はこの値を読まない・書かない。
- 直したら `make lint`。`PORTAL_RESULT_CMD` で 1 回試す。
