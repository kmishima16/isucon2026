# 再開

TTL 切れで練習環境が消えたあと、前回の最終コミットの状態に戻す手順です。

## 手順

1. 人間が `make up` を実行する（ターミナルで AWS にログインしておき、.envrc の `AWS_PROFILE` がそのプロファイルを指していること）。
2. `direnv exec . make setup` で、計測ツールと pprotein-agent を入れ直す。
3. `direnv exec . make deploy` で、前回の最終コミットの状態に戻す。sql/*.sql もここで流れる。
4. `direnv exec . make doctor` で、NG が無いことを確かめる。
5. 画面で計測を見るなら `make pp-up`。
6. hooks が読み込んだ最新の results の「次の候補」から、施策を続ける（CLAUDE.md の出来事ごとの手順 2）。

## 注意

- 練習環境はインスタンスごとにホスト鍵が変わる。isu.env の `SSH_EXTRA_OPTS`（`StrictHostKeyChecking=accept-new`）で受け入れる。
- 新しい環境はデータも初期状態に戻っている。ベンチの前に、reboot-test の行数の比較は意味を持たない（最初のベンチの後に行う）。
