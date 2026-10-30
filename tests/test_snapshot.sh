#!/usr/bin/env bash
# scripts/snapshot.sh が、初期化のメソッドと curl の追加オプション（Host・HTTPS の証明書など）を渡すかを、ssh と curl の代役で確かめる
source "$(dirname "$0")/helpers.sh"

make_fixture
mkdir -p stub
CALLS="$FIX/calls"; : > "$CALLS"
# ssh の代役：最後の引数（サーバーで動かすコマンド）を手元で動かす
cat > stub/ssh <<'EOF'
#!/usr/bin/env bash
bash -c "${!#}"
EOF
# curl の代役：引数を 1 行に残し、-w があればステータスを、無ければ本文を返す
cat > stub/curl <<'EOF'
#!/usr/bin/env bash
echo "curl $*" >> "$CALLS"
case "$*" in *"-w"*) echo "200 text/html" ;; *) echo "body" ;; esac
EOF
chmod +x stub/ssh stub/curl
cat >> isu.env <<'EOF'
SNAPSHOT_URLS="/"
SNAPSHOT_INIT="/api/initialize"
SNAPSHOT_INIT_METHOD="POST"
SNAPSHOT_BASE="https://localhost"
SNAPSHOT_CURL_OPTS="-k -H 'Host: pipe.u.isucon.local'"
EOF
PATH="$FIX/stub:$PATH" CALLS="$CALLS" bash scripts/snapshot.sh snapshot >/dev/null 2>&1
init="$(grep '/api/initialize' "$CALLS")"
contains "初期化を指定のメソッドで呼ぶ" "-X POST" "$init"
contains "初期化にも追加オプションを渡す" "Host: pipe.u.isucon.local" "$init"
page="$(grep -v '/api/initialize' "$CALLS" | head -1)"
contains "ページにも追加オプションを渡す" "Host: pipe.u.isucon.local" "$page"
contains "HTTPS の基準 URL で取る" "https://localhost/" "$page"
case "$page" in *"-X "*) ng "ページは GET のまま（-X を付けない）" ;; *) ok "ページは GET のまま（-X を付けない）" ;; esac
finish_fixture

report
