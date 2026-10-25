#!/usr/bin/env bash
# サーバー上で root として実行される（deploy.sh が送り込む）。
#   $1 = 送り込まれた設定ファイルの置き場（中身は絶対パスのミラー: etc/nginx/... など）
#   $2 = 置き換え前のファイルを退避する場所
# 変わったファイルだけを反映し、nginx / MySQL が起動できない設定なら自動で元に戻して失敗する。
set -euo pipefail
STAGE="$1"; BK="$2"; DEST="${DEST_ROOT:-}"   # DEST_ROOT はテスト用

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
[ -n "$DEST" ] && exit 0   # テスト時はここまで

if touched etc/systemd/; then systemctl daemon-reload; fi
if touched etc/sysctl; then sysctl --system >/dev/null || echo "  [warn] sysctl --system でエラー"; fi

if touched etc/nginx/ && command -v nginx >/dev/null; then
  if ! nginx -t 2>&1; then restore; nginx -t >/dev/null 2>&1 && echo "  nginx は元の設定で無事です"; exit 3; fi
  systemctl reload nginx
fi

if touched etc/mysql/; then
  for db in mysql mariadb; do
    systemctl list-unit-files --type=service --no-legend "$db.service" | grep -q . || continue
    if ! systemctl restart "$db"; then
      journalctl -u "$db" -n 15 --no-pager || true
      restore; systemctl restart "$db" && echo "  $db は元の設定で起動しました"
      exit 4
    fi
    break
  done
fi
echo "  退避先: $BK"
