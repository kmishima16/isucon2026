# pprotein

計測（alp・スロークエリの集計・pprof）を、ブラウザの画面とファイルの両方に残します。

## 役割

- ベンチごとの alp・slp・pprof を、画面（http://localhost:9000）と pprotein/data/ に残す。Claude はファイルか API（/api/httplog・/api/slowlog・/api/pprof）を読む。
- アクセスログは LTSV で出す（server/all/etc/nginx/conf.d/isu-log.conf。setup の後の deploy で反映される）。
- `make pp-up` は、Mac で pprotein を起動し、各台への ssh トンネルを張る（初回は pprotein・alp・slp を .tools/ にダウンロード）。`make pp-status`・`make pp-down` で状態を見る・止める。
- PostgreSQL の問題では、isu.env を `SLOW_LOG_KIND="pg"` にし、`SLOW_LOG` に PostgreSQL の本体のログを書く。pprotein v1.2.4 は slp を `slp my` で呼ぶと決め打ちしているので、pp-up が `.tools/slp` を scripts/slp-pg.sh への入口に差し替え、`slp pg` で読ませる（本物は `.tools/slp-bin`）。アプリ（pgx）の拡張問い合わせのログは parse・bind・execute の 3 行に分かれるので、execute の時間だけを数える。slp pg は 1 件を 1 行でしか読まないので、複数行の SQL は続きの行を 1 行につないで渡す（つながないと、複数行で書いたクエリが表から落ちる）。pprotein の画面は、slp の出力を行ごとにカンマで区切って表にする（`--format tsv` を渡すのに）。なので pprotein から呼ばれたときは、各列を "..." で囲んだ csv に直して返し、SQL の " は消す。DB の側は `log_min_duration_statement = 0` と、`SLOW_LOG_PREFIX` と同じ `log_line_prefix` が要る。どちらも reload で効く。
- `SLOW_LOG` が空なら、slowlog は収集先に入れない。
- 各台の pprotein-agent は setup が入れる。認証が無いので、systemd の `IPAddressAllow=localhost` で、その台の localhost（ssh トンネルの出口）からしか受けない。
- Mac の pprotein も認証が無く、Mac の全インターフェースで待ち受ける。外の Wi-Fi では `make pp-down` しておく。
- worktree（調査役の下ごしらえの場所）では断る（docs/kit/deploy.md）。

## いつ使うか

- 画面で見たいとき：bench の前に `make pp-up`。以降は prebench が収集を始める（`make pp-collect` で手でも始められる）。収集の長さは `PPROTEIN_SECONDS`（ポータルのベンチでは `PORTAL_MEASURE_SECONDS`）で、収集のたびに収集先を登録し直して合わせる。
- 本番の最終盤：`make pp-agent-off`（再起動しても起きない）。戻すときは `make pp-agent-on`。

## 動かし方

- scripts/pprotein.sh の冒頭のコメント、`make help`。

## 確かめ方

- 画面の group に、httplog・slowlog・pprof が Ready で並ぶ。slowlog は数百 MB になり、届くまで少しかかる。
- 2026-10 に private-isu で、v1.2.4 の 4 種類（httplog・slowlog・pprof・fgprof）の収集を確かめた。
- 2026-10 に ISUCON2026（PostgreSQL 16）で、scripts/slp-pg.sh が execute の行を集計することを、手元のログの見本で確かめた。agent の `PPROTEIN_SLOWLOG` は setup が書くので、`SLOW_LOG` を変えたら `make setup` をやり直す。
- Failed が並ぶとき（pprotein のログは pprotein/run/server.log）：
  - `make pp-up` は、:9000 で応答する pprotein を自分のものとみなし、起動を省く。ほかのリポジトリの pprotein が動いていると、データと設定がそちらに入る。先にそのリポジトリで `make pp-down` する。
  - 台を再起動すると（reboot-test を含む）、ssh トンネルが切れたまま残り、以後の収集がすべて Failed になる。`make pp-up` で張り直す（`make pp-status` でトンネルの生死を見られる）。
- `make pp-agent-on` の後は、各台の `systemctl is-active pprotein-agent` が active かを見る。「:19000 address already in use」で落ち続け、failed のまま残ったことがある。1 分ほど待ち、台の上で `sudo systemctl reset-failed pprotein-agent` をしてから、もう一度 `make pp-agent-on` する。
- `pprotein-agent --help` は、引数を読まずに agent を起動してしまう（systemd の接続元の制限なしで 19000 番を待ち受ける）。版は /usr/local/share/pprotein-agent.version で見る。

## 更新のしかた

- 版・ポート・秒数：scripts/lib.sh の `PPROTEIN_*`（変えるときは isu.env に書く）。使わないなら isu.env に `PPROTEIN_AGENT_PORT=""` と書く。
- 集める pprof：isu.env の `PPROTEIN_PPROF_PORT`・`PPROTEIN_PPROF_PATHS`。`PPROTEIN_PPROF_PORT` や `HOSTS` を後から変えたら、`make pp-up` を流し直す（トンネルを張るのは pp-up だけ）。
- 直したら `make lint`。`make pp-up` → bench で、画面に出ることを見る。
