#!/usr/bin/env bash
# 計測（nginx のアクセスログ・DB のログと集計・アプリの pprof）の切り替えと確認。
#   make measure-off  : すべて外す設定に書き換えて commit → deploy（最終計測の前に）
#   make measure-on   : すべて戻す設定に書き換えて commit → deploy
#   make measure-show : 各台で今どうなっているかを表示（リポジトリの設定と、サーバーの実際の状態）
# pprof はアプリ側で ISU_MEASURE=0 のときに待ち受けないよう作っておく（systemd の drop-in で渡す）。
# DB は、リポジトリにある方の設定だけを書き換える（MySQL の isu-slow.cnf、PostgreSQL の conf.d/isu-measure.conf）。
source "$(dirname "$0")/lib.sh"

NGINX_CONF="server/all/etc/nginx/conf.d/isu-log.conf"
MYSQL_CONF="server/all/etc/mysql/conf.d/isu-slow.cnf"
# PostgreSQL の台と版は問題ごとに違うので、server/ の下から探す（空白を含まないパスを空白区切りで持つ）
PG_CONFS=""
for f in server/*/etc/postgresql/*/main/conf.d/isu-measure.conf; do
  if [ -f "$f" ]; then PG_CONFS="$PG_CONFS $f"; fi
done
DROPIN="server/all/etc/systemd/system/$SERVICE.service.d/isu-measure.conf"
PPROF_PORT="${PPROTEIN_PPROF_PORT:-6060}"

# 1 なら計測あり、0 なら計測なし、- ならリポジトリにその設定が無い（その DB を使わない問題）
repo_state() {
  local n=1 m=- p=- a=1 f
  grep -Eq '^[[:space:]]*access_log[[:space:]]+off;' "$NGINX_CONF" && n=0
  if [ -f "$MYSQL_CONF" ]; then
    m=1; grep -Eq '^[[:space:]]*slow_query_log[[:space:]]*=[[:space:]]*0' "$MYSQL_CONF" && m=0
  fi
  for f in $PG_CONFS; do
    p=1; grep -Eq '^[[:space:]]*log_min_duration_statement[[:space:]]*=[[:space:]]*-1' "$f" && p=0
  done
  [ -f "$DROPIN" ] && grep -q 'ISU_MEASURE=0' "$DROPIN" && a=0
  echo "nginx=$n mysql=$m pg=$p app=$a"
}

switch() {   # 1 か 0
  local on="$1" label track io min f
  [ "$on" = 1 ] && label="戻す" || label="外す"
  [ -z "$(git status --porcelain)" ] || die "未コミットの変更があります。commit してから measure-$([ "$on" = 1 ] && echo on || echo off) を実行してください"
  if [ "$on" = 1 ]; then
    sed -i.bak -E "s|^([[:space:]]*)access_log[[:space:]]+off;|\1access_log $NGINX_LOG isu_ltsv;|" "$NGINX_CONF"
  else
    sed -i.bak -E "s|^([[:space:]]*)access_log[[:space:]]+$NGINX_LOG[[:space:]]+isu_ltsv;|\1access_log off;|" "$NGINX_CONF"
  fi
  rm -f "$NGINX_CONF.bak"
  if [ -f "$MYSQL_CONF" ]; then
    sed -i.bak -E "s|^([[:space:]]*slow_query_log[[:space:]]*=[[:space:]]*)[01]|\1$on|" "$MYSQL_CONF"
    rm -f "$MYSQL_CONF.bak"
  fi
  # PostgreSQL は reload で効く 3 つだけを切り替える（pg_stat_statements の読み込みは restart が要るので残す）
  if [ "$on" = 1 ]; then track=all; io=on; min=0; else track=none; io=off; min=-1; fi
  for f in $PG_CONFS; do
    sed -i.bak -E \
      -e "s|^([[:space:]]*pg_stat_statements\.track[[:space:]]*=[[:space:]]*)[a-z]+|\1$track|" \
      -e "s|^([[:space:]]*track_io_timing[[:space:]]*=[[:space:]]*)[a-z]+|\1$io|" \
      -e "s|^([[:space:]]*log_min_duration_statement[[:space:]]*=[[:space:]]*)-?[0-9]+[a-z]*|\1$min|" "$f"
    rm -f "$f.bak"
  done
  mkdir -p "$(dirname "$DROPIN")"
  printf '# make measure-on / measure-off が書き換える。0 のときアプリは pprof を待ち受けない\n[Service]\nEnvironment=ISU_MEASURE=%s\n' "$on" > "$DROPIN"
  info "リポジトリの設定: $(repo_state)"
  if [ -z "$(git status --porcelain)" ]; then info "すでに計測を${label}設定です"; else
    # shellcheck disable=SC2086  # PG_CONFS は空白区切りのパスの並び
    git add "$NGINX_CONF" "$DROPIN" $PG_CONFS
    if [ -f "$MYSQL_CONF" ]; then git add "$MYSQL_CONF"; fi
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
  if [ "$SLOW_LOG_KIND" = pg ]; then
    # PostgreSQL の MYSQL_CMD は psql を包んでいて後ろの引数を渡せないので、SQL は標準入力で渡す
    v="$(echo "SELECT 'log_min_duration_statement=' || current_setting('log_min_duration_statement') || ' pg_stat_statements.track=' || coalesce(current_setting('pg_stat_statements.track', true), '読み込みなし') || ' track_io_timing=' || current_setting('track_io_timing') AS measure;" \
      | rsh "$DB_HOST" "$MYSQL_CMD" 2>/dev/null | grep -o 'log_min_duration_statement=.*')" || v=""
    printf '  [%s] pg      %s\n' "$DB_HOST" "${v:-取得できません（MYSQL_CMD を確認）}"
  else
    v="$(rsh "$DB_HOST" "$MYSQL_CMD -N -e 'SELECT CONCAT(\"slow_query_log=\", @@slow_query_log, \" long_query_time=\", @@long_query_time)'" 2>/dev/null)"
    printf '  [%s] mysql   %s\n' "$DB_HOST" "${v:-取得できません（MYSQL_CMD を確認）}"
  fi
}

case "${1:-show}" in
  on) refuse_in_worktree measure-on; switch 1 ;;
  off) refuse_in_worktree measure-off; switch 0 ;;
  show) show ;;
  *) die "使い方: measure.sh on|off|show" ;;
esac
