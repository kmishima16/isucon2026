# local

手元の docker compose で、アプリを動かします。

## 役割

- local/compose.yml で、MySQL・memcached・アプリ（作業ツリーのコードからビルド）・nginx を動かす。
- `make local-seed` は、サーバーから部分ダンプ（`LOCAL_SEED`）と静的ファイルを取る。部分ダンプは local/data/（git に入れない）、静的ファイルは local/public/（commit する）に置く。
- `make local-up` は、部分ダンプのあとに sql/*.sql を流す（インデックスの重複は適用済みとみなす）。nginx は、サーバーと同じサイト設定を、置き場所とアプリの宛先だけ差し替えて使う。
- `make local-down` は、止めて DB を消す。次の local-up で作り直す。
- compose.yml とアプリの環境変数名は、問題ごとに書き直す（private-isu の例が入っている）。本番の 8 時間で作り直す余裕は少ないので、主に練習で使う。

## いつ使うか

- サーバーを使わずに、画面や操作を手元で確かめたいとき。巡回（docs/kit/patrol.md）の前に local-up。

## 動かし方

- scripts/local.sh の冒頭のコメント、local/compose.yml、`make help`。

## 確かめ方

- 最後に「起動しました: http://localhost:8080/」が出る。
- 起動に失敗したら、表示されるアプリのログと、`docker compose -f local/compose.yml logs mysql` を見る。

## 更新のしかた

- 部分ダンプの範囲・静的ファイルの元・サイト設定：isu.env の `LOCAL_SEED`・`LOCAL_DB_NAME`・`LOCAL_PUBLIC_REMOTE`・`LOCAL_NGINX_SITE`。ポートは scripts/lib.sh の `LOCAL_PORT`。
- サービスの構成と、アプリの環境変数：local/compose.yml。
- 直したら `make lint`。`make local-down` → `make local-up` → `make patrol` を通す。
