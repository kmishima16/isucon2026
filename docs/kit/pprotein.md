# pprotein

計測（alp・スロークエリの集計・pprof）を、ブラウザの画面とファイルの両方に残します。

## 役割

- ベンチごとの alp・slp・pprof を、画面（http://localhost:9000）と pprotein/data/ に残す。Claude はファイルか API（/api/httplog・/api/slowlog・/api/pprof）を読む。
- アクセスログは LTSV で出す（server/all/etc/nginx/conf.d/isu-log.conf。setup の後の deploy で反映される）。
- `make pp-up` は、Mac で pprotein を起動し、各台への ssh トンネルを張る（初回は pprotein・alp・slp を .tools/ にダウンロード）。`make pp-status`・`make pp-down` で状態を見る・止める。
- 各台の pprotein-agent は setup が入れる。認証が無いので、systemd の `IPAddressAllow=localhost` で、その台の localhost（ssh トンネルの出口）からしか受けない。
- Mac の pprotein も認証が無く、Mac の全インターフェースで待ち受ける。外の Wi-Fi では `make pp-down` しておく。
- worktree（調査役の下ごしらえの場所）では断る（docs/kit/deploy.md）。

## いつ使うか

- 画面で見たいとき：bench の前に `make pp-up`。以降は prebench が収集を始める（`make pp-collect` で手でも始められる）。
- 本番の最終盤：`make pp-agent-off`（再起動しても起きない）。戻すときは `make pp-agent-on`。

## 動かし方

- scripts/pprotein.sh の冒頭のコメント、`make help`。

## 確かめ方

- 画面の group に、httplog・slowlog・pprof が Ready で並ぶ。slowlog は数百 MB になり、届くまで少しかかる。
- 2026-10 に private-isu で、v1.2.4 の 4 種類（httplog・slowlog・pprof・fgprof）の収集を確かめた。

## 更新のしかた

- 版・ポート・秒数：scripts/lib.sh の `PPROTEIN_*`（変えるときは isu.env に書く）。使わないなら isu.env に `PPROTEIN_AGENT_PORT=""` と書く。
- 集める pprof：isu.env の `PPROTEIN_PPROF_PORT`・`PPROTEIN_PPROF_PATHS`。
- 直したら `make lint`。`make pp-up` → bench で、画面に出ることを見る。
