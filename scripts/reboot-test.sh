#!/usr/bin/env bash
# 再起動試験（本番の追試と同じ観点）：全台を再起動し、何もせずに動くかを見る。
#   1. 再起動の前後で、DB の全テーブルの行数が変わらないこと（書き込んだデータが残る）
#   2. 再起動後に HEALTH_URL が中身のある 200 を返すこと
#   3. 起きる順が逆でも動くこと：DB を止めたままアプリを起こし、あとから DB を起こしても、アプリが自力で戻ること
#      （再起動ではアプリが DB より先に起きることがある。DB_LATE=0 で省略）
# ベンチで書き込んだあとに実行すると、1 の確認が意味を持つ。
source "$(dirname "$0")/lib.sh"
acquire_lock "reboot-test"
T="$(ts)"

count_rows() {
  rsh "$DB_HOST" "bash -s" <<EOF
set -o pipefail
q() { $MYSQL_CMD -N -e "\$1" 2> >(grep -v 'password on the command line' >&2); }
q "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name" |
  while read -r t; do printf '%s\t%s\n' "\$t" "\$(q "SELECT COUNT(*) FROM \$t")"; done
EOF
}
db_unit() {   # DB の台で動いている DB のサービス名
  local u; for u in mysql mariadb; do has_unit "$DB_HOST" "$u" && { echo "$u"; return; }; done
}
# 中身のある 200 が返るまで待つ（DB に届かないと空の 200 を返すアプリがあるため）
wait_healthy() {   # 台 秒
  local i size
  for i in $(seq 1 "$2"); do
    size="$(rsh "$1" "curl -fsS -o /dev/null -w '%{size_download}' --max-time 5 '$HEALTH_URL'" 2>/dev/null || echo 0)"
    [ "${size:-0}" -gt 0 ] && return 0
    sleep 1
  done
  return 1
}

info "再起動前の行数（${DB_HOST}）"
count_rows > "$LOG_DIR/$T-rows-before.tsv" || die "行数を数えられません（MYSQL_CMD を確認）"
sed 's/^/  /' "$LOG_DIR/$T-rows-before.tsv"

for h in $HOSTS; do info "[$h] reboot"; rsh "$h" "sudo systemctl reboot" || true; ssh_reset "$h"; done
sleep 20
for h in $HOSTS; do
  for i in $(seq 1 60); do
    rsh "$h" true 2>/dev/null && break
    [ "$i" = 60 ] && die "[$h] 5分たっても ssh できません（make doctor）"
    sleep 5
  done
  info "[$h] 復帰"
done
for h in $APP_HOSTS; do
  wait_healthy "$h" 60 || die "[$h] 再起動後に $HEALTH_URL が中身のある応答を返しません。サービスの enable 漏れを確認（make doctor）"
  rsh "$h" "systemctl is-enabled $SERVICE" >/dev/null || warn "[$h] $SERVICE が enable されていません"
done

info "再起動後の行数"
count_rows > "$LOG_DIR/$T-rows-after.tsv" || die "行数を数えられません"
if diff -u "$LOG_DIR/$T-rows-before.tsv" "$LOG_DIR/$T-rows-after.tsv" > "$LOG_DIR/$T-rows-diff.txt"; then
  info "全テーブルの行数が再起動の前後で同じです"
else
  sed 's/^/  /' "$LOG_DIR/$T-rows-diff.txt"
  die "再起動で行数が変わりました（書き込みが残っていない。DB の永続化や、起動時に初期化していないか確認）"
fi

if [ "${DB_LATE:-1}" = 1 ]; then
  db="$(db_unit)"
  if [ -z "$db" ]; then warn "$DB_HOST に mysql / mariadb のサービスが無いので、起きる順の試験は省きます"; else
    info "起きる順の試験：$db を止めたまま $SERVICE を起こし、15 秒後に $db を起こす"
    rsh "$DB_HOST" "sudo systemctl stop $db"
    for h in $APP_HOSTS; do rsh "$h" "sudo systemctl restart $SERVICE"; done
    sleep 15
    rsh "$DB_HOST" "sudo systemctl start $db"
    for h in $APP_HOSTS; do
      wait_healthy "$h" 60 || die "[$h] DB があとから起きると $SERVICE が戻りません（起動時に DB へつなげず終了していないか、つなぎ直すか確認）"
      info "[$h] DB があとから起きても戻りました"
    done
  fi
fi

rm -rf "$LOCK_DIR"; trap - EXIT
info "全台復帰。続けて make bench で、再起動前と同じスコア帯が出るかを確認してください"
