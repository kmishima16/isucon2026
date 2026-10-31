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
  # psql の代役：PG_DOWN=1 なら問い合わせに失敗し、それ以外は PG_ROWS を pg_file_settings の結果として返す
  cat > "$W/bin/runuser" <<'EOF'
#!/usr/bin/env bash
echo "runuser $*" >> "$CALLS"
if [ "${PG_DOWN:-}" = 1 ]; then echo "psql: error: connection refused" >&2; exit 2; fi
if [ -n "${PG_ROWS:-}" ]; then printf '%s\n' "$PG_ROWS"; fi
EOF
  chmod +x "$W/bin/systemctl" "$W/bin/nginx" "$W/bin/runuser"
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

echo "PostgreSQL の設定を変えたとき"
PG_FILE=etc/postgresql/16/main/conf.d/x.conf
setup_stubs
put "$PG_FILE" "old" "new"
if UNITS="postgresql" ENABLED="postgresql" ACTIVE="postgresql" run_overlay; then ok "誤りが無ければ通る"; else ng "誤りが無ければ通る"; fi
check "誤りが無ければ reload する（restart はしない）" "systemctl reload postgresql" "$(grep -E 'reload|restart' "$CALLS" | grep postgresql)"
rm -rf "$W"
setup_stubs
put "$PG_FILE" "old" "new"
if UNITS="postgresql" ENABLED="postgresql" ACTIVE="postgresql" PG_ROWS="error /etc/postgresql/16/main/conf.d/x.conf:1 log_min_duration_statement setting could not be applied" run_overlay; then
  ng "書き間違いがあれば失敗で終わる"; else ok "書き間違いがあれば失敗で終わる"; fi
check "書き間違いがあれば設定を元に戻す" "old" "$(cat "$W/dest/$PG_FILE")"
check "書き間違いがあれば reload しない" "" "$(grep 'reload postgresql' "$CALLS")"
contains "書き間違いの場所を出す" "x.conf:1 log_min_duration_statement" "$(cat "$W/out")"
rm -rf "$W"
setup_stubs
put "$PG_FILE" "old" "new"
UNITS="postgresql" ENABLED="postgresql" ACTIVE="postgresql" PG_ROWS="restart /etc/postgresql/16/main/conf.d/x.conf:1 shared_buffers setting could not be applied" run_overlay
contains "restart が要る項目は警告を出す" "restart するまで効かない設定があります" "$(cat "$W/out")"
check "restart が要る項目があっても、ほかの項目のために reload する" "systemctl reload postgresql" "$(grep 'reload postgresql' "$CALLS")"
check "restart が要る項目があっても、設定は新しいまま" "new" "$(cat "$W/dest/$PG_FILE")"
rm -rf "$W"
setup_stubs
put "$PG_FILE" "old" "new"
if UNITS="postgresql" ENABLED="postgresql" ACTIVE="postgresql" PG_DOWN=1 run_overlay; then ng "問い合わせられなければ失敗で終わる"; else ok "問い合わせられなければ失敗で終わる"; fi
contains "問い合わせられなければ、黙らずにそう出す" "PostgreSQL に問い合わせられない" "$(cat "$W/out")"
rm -rf "$W"
setup_stubs
put "$PG_FILE" "old" "new"
UNITS="postgresql" ENABLED="" ACTIVE="" run_overlay
check "役割で止めた（disable の）postgresql は問い合わせも reload もしない" "" "$(grep -E 'runuser|reload' "$CALLS")"
rm -rf "$W"
setup_stubs
put "$PG_FILE" "old" "new"
if UNITS="postgresql" ENABLED="postgresql" ACTIVE="postgresql" FAIL_ON="reload postgresql" run_overlay; then ng "reload に失敗したら失敗で終わる"; else ok "reload に失敗したら失敗で終わる"; fi
check "reload に失敗したら設定を元に戻す" "old" "$(cat "$W/dest/$PG_FILE")"
rm -rf "$W"

report
