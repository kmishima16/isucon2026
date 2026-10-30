# local

手元の docker compose で、アプリを動かします。

## 役割

- local/compose.yml で、MySQL・memcached・アプリ（作業ツリーのコードからビルド）・nginx を動かす。
- `make local-seed` は、サーバーから部分ダンプ（`LOCAL_SEED`）と静的ファイルを取る。部分ダンプは local/data/（git に入れない）、静的ファイルは local/public/（commit する）に置く。ダンプはサーバーの側で圧縮して送り、書き終えてから置き換える（途中で失敗したら残さない）。`LOCAL_PUBLIC_REMOTE` が空なら、静的ファイルは取らない。サーバーに ssh するので、worktree では断る。
- 手元の環境の名前（compose のプロジェクト名）は、`isu-<リポジトリ名>` にする。練習と本番のリポジトリで、環境と DB のボリュームが混ざらない。ポートが重なるときは、isu.env の `LOCAL_PORT` を変える。
- MySQL の健康診断には、初期データを入れる間の猶予（`start_period: 30m`）がある。大きなダンプでも、取り込みの途中で失敗扱いにしない。
- 退避されるのは `LOCAL_DB_NAME` の 1 つの DB だけ。SQLite やファイルに持つデータは入らない。
- `make local-up` は、部分ダンプのあとに sql/*.sql を流す（インデックスの重複は適用済みとみなす）。nginx は、サーバーと同じサイト設定を、置き場所とアプリの宛先だけ差し替えて使う。
- `make local-down` は、止めて DB を消す。次の local-up で作り直す。
- compose.yml とアプリの環境変数名は、問題ごとに書き直す（private-isu の例が入っている）。新しい問題では、検証役（`/isu-local`）が書き直す。取り込んだコードに compose があれば、それを使う。
- 画面と API の対応を確かめるための環境で、性能の判断には使わない（ベンチは回さない）。中身はアプリと MySQL だけにし、画面の静的ファイルを返すのに要るときだけ nginx を入れる。付随のサービス（PowerDNS など）は入れない。

## いつ使うか

- 新しい問題の立ち上げ：操作役が `LOCAL_SEED` に全テーブルを並べて `make local-seed` を実行する（初期データの退避を兼ねる）。検証役がその後に compose を書き直し、`make local-up` で立てる。30 分ほどで立たなければ、やめる（docs/flow/contest.md の「道具が使えないとき」）。
- サーバーを使わずに、画面や操作を手元で確かめたいとき。巡回（docs/kit/patrol.md）の前に local-up。

## 動かし方

- scripts/local.sh の冒頭のコメント、local/compose.yml、`make help`。

## 確かめ方

- 最後に「起動しました: http://localhost:8080/」が出る。
- 起動に失敗したら、表示されるアプリのログと、`docker compose -p isu-<リポジトリ名> -f local/compose.yml logs mysql` を見る。`docker ps` で、ほかのリポジトリの環境が同じポートを使っていないかも見る。

## 更新のしかた

- 部分ダンプの範囲・静的ファイルの元・サイト設定：isu.env の `LOCAL_SEED`・`LOCAL_DB_NAME`・`LOCAL_PUBLIC_REMOTE`・`LOCAL_NGINX_SITE`。ポートは scripts/lib.sh の `LOCAL_PORT`。
- サービスの構成と、アプリの環境変数：local/compose.yml。
- 直したら `make lint`。`make local-down` → `make local-up` → `make patrol` を通す。
