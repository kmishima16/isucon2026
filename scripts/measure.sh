#!/usr/bin/env bash
# 計測（nginx のアクセスログ・MySQL のスローログ・アプリの pprof）の切り替えと確認。
#   make measure-off  : 3つとも外す設定に書き換えて commit → deploy（最終計測の前に）
#   make measure-on   : 3つとも戻す設定に書き換えて commit → deploy
#   make measure-show : 各台で今どうなっているかを表示（リポジトリの設定と、サーバーの実際の状態）
# pprof はアプリ側で ISU_MEASURE=0 のときに待ち受けないよう作っておく（systemd の drop-in で渡す）。
source "$(dirname "$0")/lib.sh"

NGINX_CONF="server/all/etc/nginx/conf.d/isu-log.conf"
MYSQL_CONF="server/all/etc/mysql/conf.d/isu-slow.cnf"
DROPIN="server/all/etc/systemd/system/$SERVICE.service.d/isu-measure.conf"
PPROF_PORT="${PPROTEIN_PPROF_PORT:-6060}"

# 1 なら計測あり、0 なら計測なし
repo_state() {
  local n=1 m=1 a=1
  grep -Eq '^[[:space:]]*access_log[[:space:]]+off;' "$NGINX_CONF" && n=0
  grep -Eq '^[[:space:]]*slow_query_log[[:space:]]*=[[:space:]]*0' "$MYSQL_CONF" && m=0
  [ -f "$DROPIN" ] && grep -q 'ISU_MEASURE=0' "$DROPIN" && a=0
  echo "nginx=$n mysql=$m app=$a"
}

switch() {   # 1 か 0
  local on="$1" label
  [ "$on" = 1 ] && label="戻す" || label="外す"
  [ -z "$(git status --porcelain)" ] || die "未コミットの変更があります。commit してから measure-$([ "$on" = 1 ] && echo on || echo off) を実行してください"
  if [ "$on" = 1 ]; then
    sed -i.bak -E "s|^([[:space:]]*)access_log[[:space:]]+off;|\1access_log $NGINX_LOG isu_ltsv;|" "$NGINX_CONF"
  else
    sed -i.bak -E "s|^([[:space:]]*)access_log[[:space:]]+$NGINX_LOG[[:space:]]+isu_ltsv;|\1access_log off;|" "$NGINX_CONF"
  fi
  sed -i.bak -E "s|^([[:space:]]*slow_query_log[[:space:]]*=[[:space:]]*)[01]|\1$on|" "$MYSQL_CONF"
  rm -f "$NGINX_CONF.bak" "$MYSQL_CONF.bak"
  mkdir -p "$(dirname "$DROPIN")"
  printf '# make measure-on / measure-off が書き換える。0 のときアプリは pprof を待ち受けない\n[Service]\nEnvironment=ISU_MEASURE=%s\n' "$on" > "$DROPIN"
  info "リポジトリの設定: $(repo_state)"
  if [ -z "$(git status --porcelain)" ]; then info "すでに計測を${label}設定です"; else
    git add "$NGINX_CONF" "$MYSQL_CONF" "$DROPIN"
    git commit -q -m "chore: 計測を${label}"
    info "commit: $(git log -1 --format='%h %s')"
  fi
  bash "$ROOT/scripts/deploy.sh"
  show
}

show() {
  info "リポジトリの設定: $(repo_state)（1=計測あり）"
  for h in $HOSTS; do
    rsh "$h" "sudo bash -s" <<EOF
if command -v nginx >/dev/null; then
  printf '  [%s] nginx   %s\n' "$h" "\$(nginx -T 2>/dev/null | grep -E '^[[:space:]]*access_log' | sed -E 's/^[[:space:]]+//' | sort -u | paste -sd' ' -)"
fi
if systemctl list-unit-files --no-legend '$SERVICE.service' | grep -q .; then
  env="\$(systemctl show -p Environment '$SERVICE' | grep -o 'ISU_MEASURE=[01]' || echo 'ISU_MEASURE 未設定')"
  code="\$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 http://localhost:$PPROF_PORT/debug/pprof/)"
  printf '  [%s] app     %s / pprof :%s → %s\n' "$h" "\$env" "$PPROF_PORT" "\${code/000/待ち受けなし}"
fi
if systemctl list-unit-files --no-legend pprotein-agent.service | grep -q .; then
  printf '  [%s] pprotein-agent %s\n' "$h" "\$(systemctl is-active pprotein-agent)"
fi
EOF
  done
  local v
  v="$(rsh "$DB_HOST" "$MYSQL_CMD -N -e 'SELECT CONCAT(\"slow_query_log=\", @@slow_query_log, \" long_query_time=\", @@long_query_time)'" 2>/dev/null)"
  printf '  [%s] mysql   %s\n' "$DB_HOST" "${v:-取得できません（MYSQL_CMD を確認）}"
}

case "${1:-show}" in
  on) switch 1 ;;
  off) switch 0 ;;
  show) show ;;
  *) die "使い方: measure.sh on|off|show" ;;
esac
