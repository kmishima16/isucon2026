#!/usr/bin/env bash
# 再起動試験（本番の追試と同じ観点）：全台を再起動し、何もせずに動くかを見る。
source "$(dirname "$0")/lib.sh"
acquire_lock "reboot-test"
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
  ok=0; for _ in $(seq 1 30); do rsh "$h" "curl -fsS -o /dev/null --max-time 3 '$HEALTH_URL'" && { ok=1; break; }; sleep 2; done
  [ $ok = 1 ] || die "[$h] 再起動後に $HEALTH_URL が応答しません。サービスの enable 漏れを確認（make doctor）"
  rsh "$h" "systemctl is-enabled $SERVICE" >/dev/null || warn "[$h] $SERVICE が enable されていません"
done
rm -rf "$LOCK_DIR"; trap - EXIT
info "全台復帰。続けて make bench で、再起動前と同じスコア帯が出るかを確認してください"
