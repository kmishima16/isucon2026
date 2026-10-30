#!/usr/bin/env bash
# scripts/local.sh を、ssh・rsync・docker の代役で確かめる（書きかけのダンプを残さない・worktree で断る・リポジトリごとの名前）
source "$(dirname "$0")/helpers.sh"

stubs() {   # 代役を $PWD/stub に置く。ssh は、SSH_FAIL=1 なら途中まで出して失敗する
  mkdir -p stub
  cat > stub/ssh <<'EOF'
#!/usr/bin/env bash
echo "ssh ${!#}" >> "$CALLS"
if [ "${SSH_FAIL:-0}" = 1 ]; then printf 'partial'; exit 1; fi
case "${!#}" in *gzip*) printf 'CREATE TABLE t (id int);\n' | gzip ;; *) printf 'CREATE TABLE t (id int);\n' ;; esac
EOF
  cat > stub/rsync <<'EOF'
#!/usr/bin/env bash
echo "rsync $*" >> "$CALLS"
EOF
  cat > stub/docker <<'EOF'
#!/usr/bin/env bash
echo "docker $*" >> "$CALLS"
exit 1
EOF
  chmod +x stub/ssh stub/rsync stub/docker
  STUB="$PWD/stub"
}
run() { PATH="$STUB:$PATH" CALLS="$CALLS" bash scripts/local.sh "$@"; }
# 代役が効かなかったときに本物のサーバーへ届かないよう、宛先を存在しない名前にする
safe_env() { printf 'HOSTS=no-such-host.invalid\nAPP_HOSTS=$HOSTS\nNGINX_HOST=$HOSTS\nDB_HOST=$HOSTS\n' >> isu.env; }

echo "local-seed"
make_fixture
cp -R "$REPO_SRC/local" "$FIX/"
safe_env; stubs; CALLS="$FIX/calls"; : > "$CALLS"
printf 'LOCAL_SEED="t"\n' >> isu.env
run seed >/dev/null 2>&1 || true
check "ダンプができる（gzip として読める）" "ok" "$(gzip -t local/data/initdb/00_seed.sql.gz 2>/dev/null && echo ok || echo ng)"
contains "サーバーの側で圧縮して転送する" "| gzip" "$(grep '^ssh ' "$CALLS" | head -1)"
rm -f local/data/initdb/00_seed.sql.gz
SSH_FAIL=1 run seed >/dev/null 2>&1 || true
check "途中で失敗したら、書きかけのダンプを残さない" "no" "$([ -e local/data/initdb/00_seed.sql.gz ] && echo yes || echo no)"
: > "$CALLS"
printf 'LOCAL_PUBLIC_REMOTE=""\n' >> isu.env
run seed >/dev/null 2>&1 || true
check "LOCAL_PUBLIC_REMOTE が空なら、静的ファイルを取らない" "" "$(grep '^rsync ' "$CALLS")"
finish_fixture

echo "worktree では local-seed を断る"
make_fixture
cp -R "$REPO_SRC/local" "$FIX/"
git add -A && git commit -qm "test: local"
git worktree add -q -b wt .claude/worktrees/wt main
cd .claude/worktrees/wt
safe_env; stubs; CALLS="$PWD/calls"; : > "$CALLS"
if out="$(run seed 2>&1)"; then ng "worktree では失敗で終わる"; else contains "worktree では理由を出して止まる" "main のチェックアウトで" "$out"; fi
check "worktree ではサーバーに ssh しない" "" "$(cat "$CALLS")"
finish_fixture

echo "compose のプロジェクト名はリポジトリごと（worktree でも同じ）"
make_fixture
cp -R "$REPO_SRC/local" "$FIX/"
mkdir -p local/data/initdb && printf 'x' | gzip > local/data/initdb/00_seed.sql.gz
mkdir -p server/all/etc/nginx/sites-available && echo 'server {}' > server/all/etc/nginx/sites-available/isucon.conf
safe_env; stubs; CALLS="$FIX/calls"; : > "$CALLS"
run up >/dev/null 2>&1 || true
contains "プロジェクト名にリポジトリの名前を使う" "-p isu-$(basename "$FIX" | tr 'A-Z' 'a-z' | sed 's/[^a-z0-9_-]/-/g')" "$(grep '^docker ' "$CALLS" | head -1)"
finish_fixture

echo "MySQL の健康診断に、初期データを入れる間の猶予がある"
contains "compose の MySQL に start_period がある" "start_period" "$(cat "$REPO_SRC/local/compose.yml")"

report
