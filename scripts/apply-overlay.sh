#!/usr/bin/env bash
# サーバー上で root として実行される（deploy.sh が送り込む）。
#   $1 = 送り込まれた設定ファイルの置き場（中身は絶対パスのミラー: etc/nginx/... など）
#   $2 = 置き換え前のファイルを退避する場所
# 変わったファイルだけを反映し、nginx / MySQL / PostgreSQL が受け付けない設定なら自動で元に戻して失敗する。
set -euo pipefail
STAGE="$1"; BK="$2"; DEST="${DEST_ROOT:-}"   # DEST_ROOT はテスト用（systemctl・nginx・runuser は、テストが PATH に置く代役を使う）

changed=()
cd "$STAGE"
while IFS= read -r -d '' f; do
  rel="${f#./}"; dst="$DEST/$rel"
  if [ -f "$dst" ] && cmp -s "$f" "$dst"; then continue; fi
  mkdir -p "$BK/$(dirname "$rel")" "$(dirname "$dst")"
  if [ -e "$dst" ]; then
    cp -a "$dst" "$BK/$rel"
    cat "$f" > "$dst"                       # 既存ファイルは所有者・権限をそのまま中身だけ替える
  else
    echo "$rel" >> "$BK/.created"
    install -m "$(stat -c %a "$f")" "$f" "$dst"
    if [[ "$rel" =~ ^home/([^/]+)/ ]] && id "${BASH_REMATCH[1]}" >/dev/null 2>&1; then
      chown "${BASH_REMATCH[1]}:" "$dst"
    elif [ -z "$DEST" ]; then
      chown root:root "$dst"
    fi
  fi
  changed+=("$rel")
done < <(find . -type f -print0 | sort -z)

if [ "${#changed[@]}" -eq 0 ]; then echo "  設定の変更なし"; exit 0; fi
printf '  変更: /%s\n' "${changed[@]}"

restore() {
  echo "  !! 元の設定に戻します"
  for rel in "${changed[@]}"; do
    if [ -e "$BK/$rel" ]; then cp -a "$BK/$rel" "$DEST/$rel"; else rm -f "$DEST/$rel"; fi
  done
}
touched() { printf '%s\n' "${changed[@]}" | grep -q "^$1"; }

if touched etc/systemd/; then systemctl daemon-reload; fi
if touched etc/sysctl; then sysctl --system >/dev/null || echo "  [warn] sysctl --system でエラー"; fi

# 複数台では、役割に合わせて nginx や MySQL を止めて disable にした台がある。disable の台では起こさない（nginx -t だけは行う）。
# enable のまま落ちているものは起こす。落ちた設定を直したコミットや、戻すための deploy で復旧できるように
if touched etc/nginx/ && command -v nginx >/dev/null; then
  if ! nginx -t 2>&1; then restore; nginx -t >/dev/null 2>&1 && echo "  nginx は元の設定で無事です"; exit 3; fi
  if ! systemctl list-unit-files --type=service --no-legend nginx.service | grep -q .; then
    echo "  [warn] nginx.service がありません。別の名前で動かしているなら、その unit を手で reload してください"
  elif ! systemctl is-enabled -q nginx; then
    echo "  nginx は disable なので reload しない（役割で止めた台）"
  elif ! systemctl reload-or-restart nginx; then
    restore; systemctl reload-or-restart nginx && echo "  nginx は元の設定で動いています"; exit 3
  fi
fi

if touched etc/mysql/; then
  for db in mysql mariadb; do
    systemctl list-unit-files --type=service --no-legend "$db.service" | grep -q . || continue
    if ! systemctl is-enabled -q "$db"; then echo "  $db は disable なので restart しない（役割で止めた台）"; break; fi
    if ! systemctl restart "$db"; then
      journalctl -u "$db" -n 15 --no-pager || true
      restore; systemctl restart "$db" && echo "  $db は元の設定で起動しました"
      exit 4
    fi
    break
  done
fi

# PostgreSQL は restart で接続が切れるので、reload だけを行う。書き間違いは reload の前に pg_file_settings
# （ディスクの設定ファイルを読み直し、項目ごとの誤りを返す）で見つけて元に戻す。restart が要る項目の変更は、警告だけ出す
pg_q() {   # この台本は標準入力から読まれているので、psql には読ませない
  (cd / && runuser -u postgres -- psql -XAtq -v ON_ERROR_STOP=1 -d postgres -c "$1" </dev/null)
}
if touched etc/postgresql/; then
  if ! systemctl list-unit-files --type=service --no-legend postgresql.service | grep -q .; then
    echo "  [warn] postgresql.service がありません。別の名前で動かしているなら、その unit を手で reload してください"
  elif ! systemctl is-enabled -q postgresql; then
    echo "  postgresql は disable なので reload しない（役割で止めた台）"
  elif ! found="$(pg_q "SELECT concat_ws(' ',
        CASE WHEN f.error LIKE '%without restarting%'
               OR (f.error = 'setting could not be applied' AND s.context = 'postmaster') THEN 'restart' ELSE 'error' END,
        f.sourcefile || ':' || f.sourceline, f.name, f.error)
      FROM pg_file_settings f LEFT JOIN pg_settings s ON s.name = f.name
      WHERE f.error IS NOT NULL")"; then
    echo "  !! PostgreSQL に問い合わせられないので、設定を確かめられません。落ちていないか確かめてください（systemctl status 'postgresql@*'）"
    exit 5
  elif grep -q '^error ' <<<"$found"; then
    grep '^error ' <<<"$found" | sed 's/^error /  /'
    restore; exit 5
  else
    if grep -q '^restart ' <<<"$found"; then
      echo "  [warn] restart するまで効かない設定があります（値の書き間違いもここに出るので、restart の前に確かめる）"
      grep '^restart ' <<<"$found" | sed 's/^restart /    /'
    fi
    if ! systemctl reload postgresql; then
      restore; systemctl reload postgresql && echo "  postgresql は元の設定で動いています"; exit 5
    fi
  fi
fi
echo "  退避先: $BK"
