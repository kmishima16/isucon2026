# patrol

Playwright で主な画面と操作を巡回し、ベンチでは気づきにくい表示崩れや 5xx・ブラウザのエラーを拾います。

## 役割

- トップ・投稿の詳細・ユーザーページ・ログイン・投稿とコメント・管理画面・登録を巡回する（private-isu の例）。
- `make patrol` は手元（make local-up）を巡回する。`make patrol TARGET=server` は、アプリの台に ssh トンネル（手元の 18080）を張って、本物を巡回する。
- 巡回の中身（patrol/tests/）は、問題ごとに書き直す。初期実装からある既知のエラーは `knownErrors` に挙げて数えない。

## いつ使うか

- 画面に関わる施策を deploy した後。compare（docs/kit/snapshot.md）が見ない、ログイン後の画面と操作を見たいとき。

## 動かし方

- scripts/patrol.sh の冒頭のコメント、`make help`。結果とスクリーンショットは patrol/test-results/。

## 確かめ方

- 最後に「N passed」が出る。失敗したテストは、patrol/test-results/ のスクリーンショットと error-context.md を見る。
- ベンチの直後は /initialize に数十秒かかる（上限は 90 秒にしてある）。

## 更新のしかた

- 巡回の中身：patrol/tests/patrol.spec.js（ユーザーは初期データのもの）。
- Playwright の版：patrol/package.json（版を固定している）。
- 直したら `make patrol` と `make patrol TARGET=server` の両方を通す。
