# 最初の 1 回

新しい問題でリポジトリを作り、初期スコアを取るまでの手順です。

キットがリポジトリの土台になります。ISUCON のアプリは、サーバー上で動いているものを取り込み、同じリポジトリに入れます（公式リポジトリを clone する必要はありません）。アプリ・設定・SQL が 1 つのコミットにそろうので、`make deploy REV=<rev>` で丸ごと戻せます。

## 手順

```bash
# 1. キットから新しいリポジトリを作る（問題ごとに 1 つ）
unzip isucon-kit.zip && mv isucon-kit practice-private-isu && cd practice-private-isu
git init -b main && git add -A && git commit -m "chore: キットを取り込む"   # docs.sh と hooks は main を前提にする

# 2. isu.env を問題に合わせる（private-isu はそのままでよい）。手元だけの環境変数は .envrc に書く
cp .envrc.example .envrc && direnv allow

# 3. 環境を作り、計測ツールを入れる
make up                       # 人間だけ。isuenv で競技 1 台 + ベンチ 1 台（TTL 4h）
make setup                    # 計測ツールの導入、apt の自動更新と snapd の停止、isu.env の SETUP_CMD

# 4. アプリを取り込む（APP_REMOTE → APP_LOCAL。APP_EXCLUDE のビルド成果物は除く）
make pull-app
git status                    # 余計なもの（バイナリ・巨大ファイル）が入っていないかを見る
git add -A && git commit -m "chore: 初期アプリを取り込む"

# 5. 設定ファイルを取り込む（いま全部やらなくてよい。変えたくなったときに都度）
make pull F=/etc/nginx/nginx.conf
make pull F=/etc/nginx/sites-available/isucon.conf
git add -A && git commit -m "chore: nginx の設定を取り込む"

# 6. 何も変えていない状態で一周させる（ここで初期スコアが docs/scores.tsv に記録される）
make deploy && make bench && make analyze

# 7. 持ち越しの issue の置き場として、GitHub に private リポジトリを作る（人間）
gh repo create <名前> --private --source . --remote origin
```

ここまで通ったら、Claude Code でこのフォルダを開きます。最初に開いたときは、フォルダの信頼の確認に同意します。同意するまで hooks（docs/kit/hooks.md）が動きません。

続けて、Claude Code で 2 つのセッションを開きます。操作役のセッションで `/isu-ops`、調査役のセッションで `/isu-research` を打ちます（CLAUDE.md の役割）。手順 3〜6 は、`/isu-ops` に任せても構いません。

## 取り込むときの注意

- 取り込んだら、編集する前にそのままコミットする。取り込んだ状態と比べて、何を変えたかが差分で見えるようにするため。元の内容のまま deploy しても何も変わらないので、6 の一周は「キットが正しく動くか」の確認にもなる。
- 取り込むのは実体のファイルにする。sites-enabled/ のようなシンボリックリンクではなく、リンク先の sites-available/ を取り込む（deploy はリンクを配らない）。
- ビルド成果物、vendor/ などの巨大ファイル、DB ダンプや画像データ、パスワードや鍵は入れない。make pull-app で入ってしまったら、isu.env の `APP_EXCLUDE` か .gitignore に足して取り除く。
- ホストごとに違う設定は、`make pull F=... H=<ホスト名>` で server/<ホスト名>/ に取り込む（複数台構成のとき）。
- /home/isucon/ 配下のファイル（env.sh など）も同じように取り込める。deploy では、所有者（isucon）を保ったまま書き戻す。
- バックアップが欲しければ、GitHub の private リポジトリを作って人間が push する（`git remote add origin ...` → `git push -u origin main`）。

## 初回に合わなければ直すところ

実サーバーでは確かめていない前提があります。合わなければ Claude に「make setup が失敗した。原因を調べて直して」と頼みます。

- ssh のユーザーが sudo でき、アプリは `APP_USER` で動く。
- `BUILD_CMD` の go が、`APP_USER` のログインシェルで見つかる。
- nginx の conf.d/*.conf が http ブロックで読み込まれ、server ブロックの側に access_log が無い。
- MySQL が /etc/mysql/conf.d/ を読み込む。
- Claude の権限は .claude/settings.json にある（ssh・make・git は許可、環境の操作と push は禁止）。もっと慎重にしたければ、ssh を ask に移す。
