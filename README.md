# isucon-kit

1 人 + Claude Code で、ISUCON 形式のチューニングをするための土台です。施策・順番・ツールは自由です。キットが受け持つのは「壊さない・戻せる・つながる」ことだけです。

この README は目次です。中身はそれぞれの文書にあります。

## 規則

- [CLAUDE.md](CLAUDE.md)：Claude への規則。前提・役割・守ること・伝え方・issue の決まり・コミットの規約。
- [docs/README.md](docs/README.md)：docs の規則。文書の種類・ファイル名・書くこと。

## 役割（Claude Code のスキルとサブエージェント）

- [操作役 /isu-ops](.claude/skills/isu-ops/SKILL.md)：候補を出し、人間が選んだ施策を実装・deploy・bench して伝える。サーバーに触れるのはこの役だけ。
- [調査役 /isu-research](.claude/skills/isu-research/SKILL.md)：採点の仕組み・仕様・コードから構造的な案を出し、大きな案を worktree で下ごしらえする。
- [reviewer](.claude/agents/reviewer.md)：施策の diff を敵対的にレビューする。
- [doc-writer](.claude/agents/doc-writer.md)：overview と retro を書く。
- [issue のテンプレート](.github/ISSUE_TEMPLATE/kaizen.md)：持ち越す改善項目（経緯・わかったこと・原因・完了の条件）。

## 場面ごとの手順

- [最初の 1 回](docs/flow/first-time.md)：新しい問題でリポジトリを作り、初期スコアを取るまで。
- [再開](docs/flow/resume.md)：TTL 切れで環境が消えたあと、前回の状態に戻す。
- [本番](docs/flow/contest.md)：ポータルの 3 台で始める手順と、最終盤（計測を外す → 再起動試験 → ベンチ）。

## 道具（make のターゲットのまとまり）

- [environment](docs/kit/environment.md)：練習環境の作成・削除（人間だけ）と、手元の環境変数（.envrc）。
- [setup](docs/kit/setup.md)：サーバーへの計測ツールの導入と、アプリ・設定ファイルの取り込み。
- [deploy](docs/kit/deploy.md)：コミットをサーバーに反映する。設定ミスは自動で戻す。
- [bench](docs/kit/bench.md)：ベンチと、結果の記録（docs/scores.tsv）。
- [analyze](docs/kit/analyze.md)：alp・スローログ・pprof・fgprof・vmstat・pidstat・memcached の集計。
- [measure](docs/kit/measure.md)：計測（アクセスログ・スローログ・pprof）を外す・戻す・確かめる。
- [snapshot](docs/kit/snapshot.md)：主なページの応答を比べる回帰チェック。
- [reboot-test](docs/kit/reboot-test.md)：再起動試験（行数の比較と、DB があとから起きる場合）。
- [pprotein](docs/kit/pprotein.md)：計測を画面とファイルで見る。
- [local](docs/kit/local.md)：手元の docker compose。
- [patrol](docs/kit/patrol.md)：Playwright で画面と操作を巡回する。
- [portal](docs/kit/portal.md)：本番のポータルの結果を取り出して記録する。
- [doctor](docs/kit/doctor.md)：診断・ログ・書き間違いの確認・自動テスト。
- [docs](docs/kit/docs.md)：積み上げる文書の状態と、docs ブランチの取り込み。
- [hooks](docs/kit/hooks.md)：Claude Code への自動の読み込み（役割の確認・ベストスコア）と、docs の取り込み漏れの確認。

## 記録と積み上げる文書

- [docs/scores.tsv](docs/scores.tsv)：ベンチ結果の記録（`make best` で上位を見る）。
- docs/overview・docs/retro：全体像（仕様メモ）・振り返り（規則は docs/README.md）。docs/design・docs/results は以前の施策の記録で、今は新しく書かない。

## ディレクトリ

```
isu.env        問題ごとに変える値（サーバー、アプリの場所、ビルド、ベンチ）
CLAUDE.md      Claude への規則
Makefile       操作の入口（make help）
scripts/       道具の中身（make X → scripts/X.sh）。道具の既定値は scripts/lib.sh、hooks は scripts/hooks/
tests/         道具の自動テスト（make test）
server/all/    全台に置く設定ファイル（絶対パスのミラー：server/all/etc/nginx/... → /etc/nginx/...）
server/<host>/ その台だけに置く設定ファイル
sql/           DB のスキーマ変更（deploy のたびに適用）
local/         手元の docker compose
patrol/        Playwright の巡回
docs/          文書と記録（規則は docs/README.md）
```
