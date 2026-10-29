# すべての操作の入口。設定は isu.env。   make help
SHELL := /bin/bash
.PHONY: help up down list doctor setup pull-app pull deploy prebench bench prof analyze score best status reboot-test measure-on measure-off measure-show pp-up pp-collect pp-status pp-down pp-agent-off pp-agent-on

help: ## このヘルプ
	@grep -E '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-12s %s\n",$$1,$$2}'

# --- 環境（人間が実行する） ---
up: ## [人間] isuenv で環境を作る（TTL=4h、変えるなら make up TTL=6h）
	. ./isu.env && isuenv up $$ENV --bench --ttl $${TTL:-4h}
down: ## [人間] 環境を消す
	. ./isu.env && isuenv down $$ENV
list: ## 稼働中の環境・残りTTL・概算コスト
	isuenv list

# --- 診断 ---
doctor: ## 何かおかしいときにまず実行（ssh・サービス・ディスク等を診断し、次の一手を表示）
	@bash scripts/doctor.sh
status: ## アプリのログ（直近50行）
	@bash -c 'source scripts/lib.sh; for h in $$APP_HOSTS; do rsh $$h "sudo journalctl -u $$SERVICE -n 50 --no-pager"; done'

# --- 準備 ---
setup: ## 新しいサーバーに計測ツールを入れ、SETUP_CMD を実行（何度でも可）
	bash scripts/setup.sh
pull-app: ## サーバーのアプリを APP_LOCAL に取り込む（最初の1回）
	bash scripts/pull.sh app
pull: ## サーバーの設定を server/ に取り込む  make pull F=/etc/nginx/nginx.conf [H=ホスト]
	bash scripts/pull.sh

# --- 反映・計測 ---
deploy: ## コミットをサーバーへ反映（未コミットなら拒否）。戻すときは make deploy REV=<rev>
	bash scripts/deploy.sh
prebench: ## 計測ログを空にする（本番でポータルからベンチする前）
	bash scripts/bench.sh prebench
bench: ## prebench → ベンチ → スコア記録（E="期待した動き" を一緒に残す。PROFILE=1 で pprof・fgprof も）
	bash scripts/bench.sh bench
prof: ## pprof の CPU プロファイルを取る（ベンチ実行中に）
	bash scripts/prof.sh
analyze: ## 直前のベンチの alp / pt-query-digest / pprof / vmstat / pidstat / memcached を集計して logs/ に保存
	bash scripts/analyze.sh
score: ## スコアを手で記録  make score S=12345 [M=メモ] [E=期待した動き]
	bash scripts/bench.sh score
best: ## 記録したスコアの上位（良かったコミットを探す）
	@awk -F'\t' 'NR>1' notes/scores.tsv | sort -t$$'\t' -k4,4nr | head -n $${N:-10} | awk -F'\t' '{printf "%-16s %-10s %-6s %10s  %s%s\n",$$1,$$2,$$3,$$4,$$5,($$6!="" ? "  （期待: "$$6"）" : "")}'
reboot-test: ## 全台を再起動して、何もせずに復帰するか確認
	bash scripts/reboot-test.sh
measure-show: ## 計測（アクセスログ・スローログ・pprof）が今どうなっているか
	bash scripts/measure.sh show
measure-off: ## 計測を外す設定に書き換えて commit → deploy（最終計測の前に）
	bash scripts/measure.sh off
measure-on: ## 計測を戻す設定に書き換えて commit → deploy
	bash scripts/measure.sh on

# --- pprotein（計測を画面とファイルで見る。Mac で動かす） ---
pp-up: ## pprotein を起動し、各台への ssh トンネルと収集先を設定（画面 http://localhost:9000）
	bash scripts/pprotein.sh up
pp-collect: ## pprotein の収集を手で始める（make prebench からも自動で呼ばれる）
	bash scripts/pprotein.sh collect
pp-status: ## pprotein とトンネルの状態、登録した収集先
	bash scripts/pprotein.sh status
pp-down: ## pprotein とトンネルを止める（データは pprotein/data/ に残る）
	bash scripts/pprotein.sh down
pp-agent-off: ## 各台の pprotein-agent を止める（最終計測の前）
	bash scripts/pprotein.sh agent-off
pp-agent-on: ## 各台の pprotein-agent を戻す
	bash scripts/pprotein.sh agent-on
