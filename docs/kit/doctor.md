# doctor

診断と、道具の書き間違いの確認を受け持ちます。

## 役割

- `make doctor` は、手元のツール・ロック・作業ツリー・デプロイ済みの版 → isuenv の環境 → ssh → 各台のサービスとディスク・メモリ・負荷、の順に確かめ、NG には次の一手を出す。
- `make status` は、アプリのログ（直近 50 行）を出す。
- `make lint` は、スクリプトの構文・変数の直後の全角文字・shellcheck（入っていれば）・README の目次と、生きた文書のリンクを、実行する前に確かめる。
- `make test` は、道具の自動テスト（tests/test_*.sh）を流す。docs.sh・記録・hooks・lint の確認を、使い捨てのリポジトリで試す。

## いつ使うか

- 何かおかしいとき：まず `make doctor`。
- 道具（scripts/・Makefile・docs/kit・docs/flow・.claude/）を変えたとき：`make lint` と `make test`（CLAUDE.md の「道具を変えるとき」）。

## 動かし方

- scripts/doctor.sh・scripts/lint.sh の冒頭のコメント、tests/、`make help`。

## 確かめ方

- doctor：「問題は見つかりませんでした」が出る。NG は上から順に直す。
- 自宅と外で IP が変わってつながらないとき：人間が `isuenv ssh <環境名>` を実行する（doctor が原因を判定して表示する）。
- TTL 切れで環境が消えたとき：doctor が見つける。人間が `make up` し、docs/flow/resume.md の手順で戻す。

## 更新のしかた

- 診断の項目：scripts/doctor.sh。lint の確認：scripts/lint.sh。テスト：tests/。
- 道具を足したら、README の目次と docs/kit に手引きを足す（lint が目次の漏れを見つける）。
