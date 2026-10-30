# deploy

コミットをサーバーに反映します。壊さないための仕組みの多くは、ここに入っています。

## 役割

- コミット済みの状態（`git archive`）だけを配る。未コミットの変更があれば止まる。何をベンチしたか分からなくなるのを防ぐため。
- 順番は、server/ の設定ファイル → アプリの転送とビルド → sql/*.sql の適用 → サービスの再起動と疎通確認。
- 変わった設定ファイルだけを置き換え、元のファイルを /var/backups/isu-deploy/<時刻>-<rev>/ に退避する。
- nginx -t の失敗や MySQL の起動失敗を見つけると、自動で元の設定に戻して止まる。
- ネットワーク・ssh・権限まわりのファイル（etc/ssh・netplan・hosts・ufw・sudoers など）は配らない。外から入れなくなるのを防ぐため。
- ビルドや疎通確認に失敗したら止まる。古いバイナリのままベンチするのを防ぐため。
- deploy とベンチは、ロック（.lock/）で直列にする。
- worktree（調査役の下ごしらえの場所）では、deploy・bench・reboot-test・snapshot を断る。ロックがチェックアウトごとにあり、すり抜けるため。
- ssh には、タイムアウト・keepalive・接続の使い回しを設定してある（scripts/lib.sh）。回線が切れても固まらない。

## いつ使うか

- 施策や道具の変更をコミットした直後（CLAUDE.md の出来事ごとの手順 4）。続けて `make compare`（docs/kit/snapshot.md）。
- 戻したいとき：`make best` で良かったコミットを探し、`make deploy REV=<rev>`。

## 動かし方

- scripts/deploy.sh の冒頭のコメント、`make help`。

## 確かめ方

- 最後に「deploy 完了: <rev> <件名>」が出る。logs/deployed に、反映したコミットが残る。
- 失敗したら、表示されたメッセージと `make status`（アプリのログ）を見る。設定は自動で戻っている。

## 更新のしかた

- 配らないファイル：scripts/deploy.sh の `FORBIDDEN`。
- 反映した後の扱い（reload・restart）：scripts/apply-overlay.sh。
- アプリの場所・ビルド・サービス名：isu.env の `APP_LOCAL`・`APP_REMOTE`・`BUILD_CMD`・`SERVICE`・`HEALTH_URL`。
- 直したら `make lint`。練習環境で `make deploy` を通す。
