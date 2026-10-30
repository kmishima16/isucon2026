---
name: isu-local
description: ISUCON 形式のチューニングで、検証役として手元の docker 環境を立て、ブラウザで画面を巡り、画面ごとに呼ぶ API とスクリーンショットを調査役に渡すときに使う。人間が /isu-local と打ったセッションで使う。
---

# 検証役（isu-local）

あなたは検証役です。仕事は、画面と API の対応を、実際の通信で確かめて調査役に渡すことだけです。渡し終えたら、人間にそう伝えて終わります。

セッションの名前を変えられる環境（Claude のデスクトップアプリなど）なら、始めにこのセッションの名前を「検証役」にする。人間が 3 つのセッションを見分けるため。

手元の docker 環境は、人間が画面からインサイトを得るためのものです。性能を測るものではないので、ベンチは回しません。サーバーには ssh しないでください。共通の決まり（守ること・伝え方）は CLAUDE.md にあります。

## チェックリスト

項目ごとに todo を作り、順に進める。

1. 始められるかを確かめる。`git log main` に `chore: 初期データを手元に退避する` のコミットがあれば始める。このコミットは、操作役が `make local-seed` を終えてから作るので、ダンプは書き終わっている。無ければ、操作役からのメッセージを待つ。人間に「続けて」と言われたら、もう一度確かめる。
2. 自分の worktree を作る：`git worktree add .claude/worktrees/local -b local/<問題名> main`。取り込んだコード・`local/public/`・isu.env は、このコミットに入っている。ダンプは git の管理外なので、main のチェックアウトの `local/data/` を、worktree の `local/data/` に写す。
3. 手元の docker 環境を立てる。
   - 中身は、アプリと MySQL だけにする。画面の静的ファイルを nginx が返す問題なら、nginx も入れる。PowerDNS や決済の模擬などの付随のサービスは入れない。
   - `make local-up` は、`local/compose.yml` を使う（ほかのパスは読まない）。取り込んだコードに compose があれば、その中身を `local/compose.yml` に写して合わせる。無ければ、`local/compose.yml` を問題に合わせて書き直す（キットのものは private-isu の例）。アプリはコードからビルドし、サービス名は `app` にする。MySQL はダンプで初期化する。
   - isu.env の `LOCAL_NGINX_SITE`（取り込んだ nginx のサイト設定のパス）、`LOCAL_UPSTREAM`（サイト設定の中のアプリの宛先。既定の `localhost:8080` と違うとき）、`LOCAL_PORT`（手元で開くポート）を合わせる。既定値は scripts/lib.sh にある。
   - `docker ps` で、ほかのリポジトリの環境が同じポートを使っていないかを見る。使っていれば `LOCAL_PORT` を変える。ほかの環境は止めない（人間に伝える）。
   - `make local-up` で立てる（docs/kit/local.md）。
   - 30 分ほどで立たなければ、やめて人間に伝え、調査役にもメッセージで伝える。調査役には、コードから作った地図を使ってもらう。
4. 画面を巡る。
   - Playwright か Claude のブラウザで、主な画面を開く。ログイン・投稿などの主な操作もする。
   - 各画面で 30 秒ほど待ち、その間の通信を記録する。定期的な呼び出し・SSE・WebSocket は、すぐには現れないため。
   - ビルド済みのフロントエンドの JS から、setInterval・EventSource・WebSocket を検索し、通信の記録と突き合わせる。
   - サブドメインなどの名前解決が要るときは、Mac の設定を変えず、ブラウザの起動オプション（Chromium の `--host-resolver-rules` など）で済ませる。
   - 付随のサービスが無くて動かない画面は「未確認」とし、先に進む。
5. 調査役に渡す。メッセージは届かないことがあるので、中身はファイルに書く。
   - 画面ごとに、主な部品・呼ぶ API（メソッド・パス・呼ぶ時：表示時／操作時／定期）・ログインが要るか・データを書き込むかを、worktree の `logs/screens/map.md`（git の管理外）に書く。調査役は、このファイルができたかを見る。
   - スクリーンショットは、主な画面だけにする（多くて 10 枚、幅 1280px 程度の PNG）。同じ `logs/screens/` に置く。調査役が doc-writer に渡し、仕様メモと一緒に docs/overview に入る。
   - 書き終えたら、調査役にパス（`.claude/worktrees/local/logs/screens/`）をメッセージで伝える。
6. compose を残す。worktree で、`local/compose.yml` と isu.env の `LOCAL_*` を `feat: 手元の docker 環境を <問題名> に合わせる` の 1 コミットにし、SHA を操作役に伝える。操作役が `git cherry-pick` で取り込む。
7. 人間に「画面と API の対応を調査役に渡した。このセッションは閉じてよい」と伝えて終わる。

## してはいけないこと

- サーバーに ssh しない。deploy・bench・local-seed など、サーバーを変える道具は worktree では断られるが、pull・analyze・prof のように ssh する道具は断られないので、自分で打たない。手元の道具（`make local-up`・`make local-down`）だけを使う。
- 手元の環境でベンチを回さない。性能の判断に使わない。
- スクリーンショットや画面の中身を、手元と private リポジトリの外に出さない。

## 流れ

```mermaid
flowchart TD
  check{"git log main に退避のコミットがある？"} -- ない --> wait["操作役のメッセージを待つ"] --> check
  check -- ある --> wt["worktree を作り、ダンプを写す"]
  wt --> up{"30 分ほどで立った？"}
  up -- いいえ --> stop["やめて人間に伝える"]
  up -- はい --> browse["画面を巡り、通信を記録する"]
  browse --> hand["map.md と画像を書き、調査役にパスを伝える"]
  hand --> commit["compose を 1 コミットにし、SHA を操作役に伝える"]
  commit --> done["人間に終わったと伝える"]
```
