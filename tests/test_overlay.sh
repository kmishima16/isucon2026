#!/usr/bin/env bash
# scripts/apply-overlay.sh を、systemctl と nginx の代役で試す。
# 役割で止めた（disable にした）サービスは起こさず、enable のまま落ちているサービスは起こす（戻す deploy で直るように）
source "$(dirname "$0")/helpers.sh"
OVERLAY="$REPO_SRC/scripts/apply-overlay.sh"

# 代役：呼ばれた引数を CALLS に残す。ENABLED にある unit を enable、ACTIVE にある unit を動いている扱いにする
setup_stubs() {
  W="$(mktemp -d)"
  mkdir -p "$W/bin" "$W/stage" "$W/dest" "$W/bk"
  CALLS="$W/calls"; : > "$CALLS"
  cat > "$W/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
echo "systemctl $*" >> "$CALLS"
listed() { local s; for s in $1; do [ "$s" = "$2" ] && return 0; done; return 1; }
case "$1" in
  is-active) listed "$ACTIVE" "${!#}" ;;
  is-enabled) listed "$ENABLED" "${!#}" ;;
  list-unit-files) for u in $UNITS; do case "$*" in *"$u.service"*) echo "$u.service enabled enabled"; esac; done; exit 0 ;;
  reload|restart|reload-or-restart) [ "${FAIL_ON:-}" = "$1 $2" ] && exit 1; exit 0 ;;
  *) exit 0 ;;
esac
EOF
  cat > "$W/bin/nginx" <<'EOF'
#!/usr/bin/env bash
echo "nginx $*" >> "$CALLS"
exit 0
EOF
  chmod +x "$W/bin/systemctl" "$W/bin/nginx"
}
put() {   # 相対パス 置き換え前の中身 置き換え後の中身
  mkdir -p "$W/dest/$(dirname "$1")" "$W/stage/$(dirname "$1")"
  printf '%s\n' "$2" > "$W/dest/$1"
  printf '%s\n' "$3" > "$W/stage/$1"
}
run_overlay() { PATH="$W/bin:$PATH" CALLS="$CALLS" DEST_ROOT="$W/dest" bash "$OVERLAY" "$W/stage" "$W/bk" > "$W/out" 2>&1; }

echo "MySQL の設定を変えたとき"
setup_stubs
put etc/mysql/conf.d/x.cnf "old" "new"
UNITS="mysql nginx" ENABLED="" ACTIVE="" run_overlay
check "役割で止めた（disable の）mysql は restart しない" "" "$(grep 'restart mysql' "$CALLS")"
rm -rf "$W"
setup_stubs
put etc/mysql/conf.d/x.cnf "old" "new"
UNITS="mysql nginx" ENABLED="mysql" ACTIVE="" run_overlay
check "enable のまま落ちている mysql は restart で起こす" "systemctl restart mysql" "$(grep 'restart mysql' "$CALLS")"
rm -rf "$W"
setup_stubs
put etc/mysql/conf.d/x.cnf "old" "new"
UNITS="mysql nginx" ENABLED="mysql" ACTIVE="mysql" run_overlay
check "動いている mysql は restart する" "systemctl restart mysql" "$(grep 'restart mysql' "$CALLS")"
rm -rf "$W"

echo "nginx の設定を変えたとき"
setup_stubs
put etc/nginx/conf.d/x.conf "old" "new"
if UNITS="mysql nginx" ENABLED="" ACTIVE="" run_overlay; then ok "役割で止めた nginx でも通る"; else ng "役割で止めた nginx でも通る"; fi
check "役割で止めた（disable の）nginx は reload しない" "" "$(grep 'reload' "$CALLS")"
check "止めてあっても設定は確かめる（nginx -t）" "nginx -t" "$(grep '^nginx -t' "$CALLS" | head -1)"
rm -rf "$W"
setup_stubs
put etc/nginx/conf.d/x.conf "old" "new"
UNITS="mysql nginx" ENABLED="nginx" ACTIVE="" run_overlay
check "enable のまま落ちている nginx は reload-or-restart で起こす" "systemctl reload-or-restart nginx" "$(grep 'reload-or-restart nginx' "$CALLS")"
rm -rf "$W"
setup_stubs
put etc/nginx/conf.d/x.conf "old" "new"
if UNITS="mysql nginx" ENABLED="nginx" ACTIVE="nginx" FAIL_ON="reload-or-restart nginx" run_overlay; then ng "reload に失敗したら失敗で終わる"; else ok "reload に失敗したら失敗で終わる"; fi
check "reload に失敗したら設定を元に戻す" "old" "$(cat "$W/dest/etc/nginx/conf.d/x.conf")"
rm -rf "$W"
setup_stubs
put etc/nginx/conf.d/x.conf "old" "new"
UNITS="mysql" ENABLED="" ACTIVE="" run_overlay
contains "nginx の unit が無い台では、黙らずにそう出す" "nginx.service がありません" "$(cat "$W/out")"
rm -rf "$W"

report
