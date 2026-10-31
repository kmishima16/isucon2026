#!/usr/bin/env bash
# europe1・america1 の nginx の振り分け（段階 1）を、手元の docker で確かめる。
# 転送先の台の代わりに、同じ nginx の中に「どこに届いたか」と「渡されたリージョン」を返す待ち受けを置く。
# 使い方：local/check-nginx-routes.sh europe1 europe ／ local/check-nginx-routes.sh america1 america
set -euo pipefail
cd "$(dirname "$0")/.."
host="${1:-europe1}"
region="${2:-europe}"
site="server/$host/etc/nginx/sites-available/isureal.conf"
name="isu-route-check-$host"
work="$(mktemp -d)"
trap 'docker rm -f "$name" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT

sed -e 's/192\.168\.1\.10:8080/127.0.0.1:9001/' -e 's/192\.168\.4\.10:8080/127.0.0.1:9002/' "$site" > "$work/site.conf"
cat > "$work/stubs.conf" <<'EOF'
server { listen 8080; location / { return 200 "local region=[$http_x_isureal_region]\n"; } }
server { listen 9001; location / { return 200 "asia1 region=[$http_x_isureal_region]\n"; } }
server { listen 9002; location / { return 200 "asia2 region=[$http_x_isureal_region]\n"; } }
EOF
docker run -d --name "$name" -p 127.0.0.1:18080:80 \
  -v "$work/site.conf:/etc/nginx/conf.d/default.conf:ro" \
  -v "$work/stubs.conf:/etc/nginx/conf.d/stubs.conf:ro" nginx:1.27.3 >/dev/null
for _ in $(seq 1 50); do curl -s -o /dev/null http://127.0.0.1:18080/ && break; sleep 0.1; done

fail=0
L='^local region=\[\]$'
A="^asia[12] region=\[$region\]\$"
check() {
  local got
  # 外からの偽のヘッダーを付けて送り、どちらの行き先でも消えるか、正しい値に置き換わることを確かめる
  got="$(curl -sS -X "$1" -H 'X-Client-IP: 203.0.113.7' -H 'X-Isureal-Region: asia' "http://127.0.0.1:18080$2")"
  if printf '%s' "$got" | grep -Eq "$3"; then echo "ok  $1 $2 -> $got"; else echo "NG  $1 $2 -> $got (want /$3/)"; fail=1; fi
}
id=0f8b2a6e-1c3d-4e5f-8a9b-0c1d2e3f4a5b
check GET    /api/notifications                  "$L"
check GET    "/api/images/$id?exp=1&sig=x"       "$L"
check POST   /api/posts                          "$L"
check DELETE "/api/posts/$id"                    "$L"
check GET    /api/posting-window/current         "$L"
check GET    /api/posts/me/current               "$L"
check POST   /api/auth/register                  "$L"
check POST   /api/auth/login                     "$L"
check POST   /api/auth/logout                    "$L"
check POST   /api/me/account-deletion            "$L"
check PATCH  /api/me                             "$L"
check GET    /api/me/export                      "$L"
check POST   /api/initialize                     "$L"
check POST   /api/admin/posting-window           "$L"
check GET    /api/feeds/friends                  "$A"
check GET    /api/memories                       "$A"
check GET    "/api/memories/2026-11-01"          "$A"
check GET    "/api/posts/$id"                    "$A"
check PATCH  "/api/posts/$id"                    "$A"
check GET    "/api/posts/$id/comments"           "$A"
check POST   "/api/posts/$id/realmojis"          "$A"
check POST   /api/friend-requests                "$A"
check POST   "/api/friend-requests/$id/accept"   "$A"
check GET    /api/relationships/friends          "$A"
check PUT    "/api/follows/$id"                  "$A"
check PUT    "/api/blocks/$id"                   "$A"
check GET    /api/users                          "$A"
check GET    /api/me                             "$A"
check PUT    /api/me/profile-image               "$A"
exit "$fail"
