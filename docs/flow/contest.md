# 本番

ポータルの台（これまでは 3 台）で始める手順と、ベンチ、道具が使えないときの置き換え、最終盤（計測を外す → 再起動試験 → ベンチ）の手順です。時刻ごとの段取りは、人間が手元のチートシートで持ち、ここには書きません。

## 始め方

- 最初の 1 回（docs/flow/first-time.md）と同じ流れで、開始直後に新しいリポジトリを作る。違うのは手順 2 と 3 だけにする。make up は使わず、当日マニュアルの方法で台を立て、~/.ssh/config に書いてから `make setup`。台数と立て方は、当日マニュアルで確かめる（ISUCON2026 は、チームの AWS アカウントに CloudFormation で立て、ssh のコマンドはポータルに出る形だった）。
- isu.env を書き換える：`ENV=""`、`HOSTS` を ~/.ssh/config に書いた台の名前に、アプリの場所・サービス名・DB の接続、`BENCH_CMD=""`（ベンチはポータルから）。
- GitHub に private リポジトリを作り（人間）、3 つのセッションを同時に開く。操作役に `/isu-ops`、調査役に `/isu-research`、検証役に `/isu-local` を打つ。台に ssh できるようになったら、操作役がアプリを取り込む前に開く。会話の fork ではなく、新しいセッションで開く（first-time.md）。
- 操作役は、isu-ops の「立ち上げ」を回す。初期データは、アプリを取り込んだ直後（deploy の前）に `make local-seed` で手元に退避する（中身は mysqldump）。最初の 3 つの施策は、コミットを分けたうえで、ポータルのベンチは 1 回だけにする（人間を呼ぶ回数を減らす）。
- 人間は、マニュアルの採点の節とレギュレーションの要点を、自分で読む。
- 練習のリポジトリの pprotein が動いていれば、先にそのリポジトリで `make pp-down` する。`make pp-up` は、:9000 で応答するものを自分のものとみなす（docs/kit/pprotein.md）。
- DB を別の台に分けるとき：MySQL の bind-address、ユーザーの接続元ホスト（'%'）、アプリの接続先を private IP にする。
- 台の役割を分けたら：止める台では、そのサービスを `systemctl disable --now` する（`SETUP_CMD` に書く。deploy は disable のサービスを起こさない）。nginx を止めた台が `APP_HOSTS` にいれば、`HEALTH_URL` をアプリのポートに向ける（docs/kit/deploy.md）。

## ベンチ

- `make prebench` → 人間がポータルでベンチを実行する → 人間が見たスコアを `make score S=<点数>` で記録する → `make analyze` の順にする。スコアの記録は、`make score S=<点数>` を既定にする。ベンチが fail していれば `PASS=false` を付ける（付けないと `make best` が拾う）。fail の件数が分かれば `F=<件数>` も付ける。
- `make portal WATCH=1` は、ポータルから結果を取り出せると分かったときだけ使う（docs/kit/portal.md）。
- 施策 1 回の流れは、操作役のスキル（.claude/skills/isu-ops/SKILL.md）に従う。

## 道具が使えないとき

本番中にキットのスクリプトを直すのは、操作役の手順が止まっていて、10 分ほどで直せるときだけにする（CLAUDE.md の「困ったら」）。それ以外は、次のように置き換えて進める。気付いたことは `logs/kit-notes.md` に一行で控え、競技が終わってから isucon-kit の issue にする（本番中に issue を書く時間を取らない）。

- ポータルから結果を取り出せない → 人間が見たスコアを `make score` で記録する。
- pprotein が動かない → 使わない。計測は `make analyze` のテキストで見る。
- snapshot や compare が動かない → 使わない。応答の確かめは、ベンチの整合性チェックに任せる。
- スローログの集計が使えない（MySQL 以外の DB など）→ alp と pprof に絞る。
- 検証役の手元の docker 環境が立たない → 検証役を閉じる。画面と API の対応は、調査役がコードから作る。
- Go などがビルドで見つからない → スクリプトは直さず、isu.env の `BUILD_CMD` に絶対パスで書く。

## 最終盤

人間に「締めて」と言われたら、操作役が次の順に進める。

1. `make measure-off`：アクセスログ・スローログ・pprof を外して commit → deploy する（docs/kit/measure.md）。
2. `make pp-agent-off`：各台の pprotein-agent を止める（再起動しても起きない）。
3. `make reboot-test`：行数の比較と、DB があとから起きる場合を確かめる（docs/kit/reboot-test.md）。再起動は順序を変えて 2 通り行い、再起動の後にデータが残っていることを確かめる。
4. 人間にポータルでベンチを頼み、スコア帯を確かめて `make score` で記録する。最終の計測は 2 回回す。
5. 競技が終わっても、追試が終わるまで EC2 を止めない。追試が終わったら、台を立てたスタック（自分の AWS アカウントのもの）を消す。消すまで課金が続く。
