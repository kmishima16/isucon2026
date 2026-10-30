#!/usr/bin/env bash
# おかしいと思ったらまずこれ。手元 → 環境 → ssh → サービス → リソース の順に確認し、次の一手を表示する。
source "$(dirname "$0")/lib.sh"
set +e
ng=0
ok()  { printf '  \033[32mok\033[0m  %s\n' "$*"; }
bad() { printf '  \033[31mNG\033[0m  %s\n' "$*"; ng=1; }

info "手元"
for c in git rsync ssh curl; do command -v $c >/dev/null && ok "$c" || bad "$c がありません"; done
if [ -n "$ENV" ]; then command -v isuenv >/dev/null && ok "isuenv" || bad "isuenv がありません"; fi
[ -d .lock ] && bad "ロックが残っています: $(cat .lock/owner 2>/dev/null)（実行中でなければ rm -rf .lock）"
h="$(git rev-parse --short HEAD 2>/dev/null)"; d="$(deployed_rev)"; [ "$d" = '?' ] && d=なし
[ -z "$(git status --porcelain)" ] && ok "作業ツリーはクリーン" || printf '  --  未コミットの変更あり（deploy 前に commit）\n'
if same_as_deployed; then ok "デプロイ済み = HEAD（docs/ だけの差は無視） ($h)"; else printf '  --  デプロイ済み %s / HEAD %s（docs/ 以外が違う）\n' "$d" "$h"; fi

if [ -n "$ENV" ] && command -v isuenv >/dev/null; then
  info "isuenv 環境"
  l="$(isuenv list 2>&1)"; echo "$l" | sed 's/^/  /'
  echo "$l" | grep -q "^$ENV[[:space:]]" || bad "環境 $ENV が見つかりません（TTL切れ？）→ 人間が make up → make setup → make deploy"
fi

info "ssh"
for host in $HOSTS $BENCH_HOST; do
  out="$(rsh "$host" true 2>&1)" || { ssh_reset "$host"; out="$(rsh "$host" true 2>&1)"; }
  if [ $? -eq 0 ]; then ok "$host"; continue; fi
  bad "$host: $(echo "$out" | tail -1)"
  case "$out" in
    *"timed out"*|*"Connection refused"*|*"No route"*)
      echo "      → IP が変わった可能性（SG は起動時のIP/32だけ許可）。人間が 'isuenv ssh $ENV' を一度実行して exit。"
      echo "        本番ならポータルの接続情報と自分の回線を確認。" ;;
    *"Could not resolve"*) echo "      → ssh config に $host が無い。環境が消えたか、isuenv ssh で config を再生成。" ;;
    *"IDENTIFICATION HAS CHANGED"*) echo "      → 同名の別インスタンス。ssh-keygen -R <IP> で古い鍵を消す。" ;;
    *"Permission denied"*) echo "      → 鍵かユーザーが違う。isuenv problems で ssh ユーザーを確認。" ;;
  esac
done

info "サービスとリソース"
for host in $HOSTS; do
  rsh "$host" true 2>/dev/null || continue
  rsh "$host" "bash -s" <<EOF
for u in $SERVICE nginx mysql mariadb memcached redis-server; do
  systemctl list-unit-files --type=service --no-legend "\$u.service" 2>/dev/null | grep -q . || continue
  printf '  [%s] %-14s %s / %s\n' "$host" "\$u" "\$(systemctl is-active \$u)" "\$(systemctl is-enabled \$u 2>/dev/null)"
done
printf '  [%s] disk / %s   mem avail %s   load %s\n' "$host" "\$(df -h / | awk 'NR==2{print \$5}')" "\$(free -h | awk '/Mem/{print \$7}')" "\$(cut -d' ' -f1-3 /proc/loadavg)"
EOF
done
for host in $APP_HOSTS; do
  rsh "$host" true 2>/dev/null || continue
  rsh "$host" "curl -fsS -o /dev/null --max-time 5 '$HEALTH_URL'" 2>/dev/null && ok "[$host] $HEALTH_URL" \
    || bad "[$host] $HEALTH_URL が応答しない → make status / make deploy REV=<良かったrev>"
done

echo
[ $ng = 0 ] && info "問題は見つかりませんでした" || warn "NG の項目を上から順に解消してください"
