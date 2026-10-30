#!/usr/bin/env bash
# 回帰チェック：主なページの応答を取っておき、施策の後に同じ状態で取り直して比べる。
#   make snapshot [NAME=base]          : 今デプロイされているアプリの応答を logs/snapshot/<NAME>/ に保存
#   make compare  [BASE=base] [NAME=current] : NAME を取り直して BASE と比べる（違えば終了コード 1）
# 取る前に SNAPSHOT_INIT（/initialize など）を叩いて、データを初期状態にそろえる。
# 対象は isu.env の SNAPSHOT_URLS。CSRF トークンなど毎回変わる部分は SNAPSHOT_IGNORE で消してから比べる。
# 問題によって初期化のメソッド（ISUCON13 は POST /api/initialize）や、Host・HTTPS が違うので、
# SNAPSHOT_INIT_METHOD と SNAPSHOT_CURL_OPTS（例: -k -H 'Host: pipe.u.isucon.local'）で合わせる。
source "$(dirname "$0")/lib.sh"
[ -n "${SNAPSHOT_URLS:-}" ] || die "isu.env の SNAPSHOT_URLS が空です"
h="${APP_HOSTS%% *}"
BASE_URL="${SNAPSHOT_BASE:-http://localhost}"
OPTS="${SNAPSHOT_CURL_OPTS:-}"
SNAP_DIR="$LOG_DIR/snapshot"

slug() { printf '%s' "$1" | sed -E 's#^/##; s#[^A-Za-z0-9._-]+#_#g; s#^$#_root#'; }

take() {   # 名前
  local dir="$SNAP_DIR/$1" u s
  rm -rf "$dir"; mkdir -p "$dir"
  if [ -n "${SNAPSHOT_INIT:-}" ]; then
    rsh "$h" "curl -fsS -o /dev/null --max-time 60 -X $SNAPSHOT_INIT_METHOD $OPTS '$BASE_URL$SNAPSHOT_INIT'" || die "$SNAPSHOT_INIT が失敗しました"
  fi
  echo "$(cut -d' ' -f1 "$LOG_DIR/deployed" 2>/dev/null || echo '?') $(date '+%F %T')" > "$dir/.rev"
  for u in $SNAPSHOT_URLS; do
    s="$(slug "$u")"
    # 本文と、ステータス・Content-Type を分けて保存する（本文は画像などバイナリもある）
    rsh "$h" "curl -sS --max-time 30 $OPTS '$BASE_URL$u'" > "$dir/$s.body"
    printf '%s\t%s\n' "$u" "$(rsh "$h" "curl -sS -o /dev/null --max-time 30 $OPTS -w '%{http_code} %{content_type}' '$BASE_URL$u'")" >> "$dir/index.tsv"
  done
  info "保存: logs/snapshot/$1/（rev $(cut -d' ' -f1 "$dir/.rev")、$(wc -l < "$dir/index.tsv" | tr -d ' ') ページ）"
}

normalize() {   # ファイル
  if [ -n "${SNAPSHOT_IGNORE:-}" ] && LC_ALL=C grep -Iq . "$1"; then sed -E "s#${SNAPSHOT_IGNORE}#<IGNORED>#g" "$1"; else cat "$1"; fi
}

compare() {   # 基準 比べる側
  local a="$SNAP_DIR/$1" b="$SNAP_DIR/$2" u s ma mb ng=0
  [ -f "$a/index.tsv" ] || die "基準 logs/snapshot/$1 がありません。先に make snapshot NAME=$1"
  take "$2"
  info "比較: $1（rev $(cut -d' ' -f1 "$a/.rev")） → $2（rev $(cut -d' ' -f1 "$b/.rev")）"
  while IFS=$'\t' read -r u ma; do
    s="$(slug "$u")"
    mb="$(awk -F'\t' -v u="$u" '$1==u{print $2}' "$b/index.tsv")"
    if [ "$ma" != "$mb" ]; then
      printf '  \033[31mDIFF\033[0m %-28s %s → %s\n' "$u" "$ma" "${mb:-（取れていない）}"; ng=1; continue
    fi
    if cmp -s <(normalize "$a/$s.body") <(normalize "$b/$s.body"); then
      printf '  \033[32msame\033[0m %-28s %s\n' "$u" "$ma"
    else
      printf '  \033[31mDIFF\033[0m %-28s 本文が違う\n' "$u"; ng=1
      if LC_ALL=C grep -Iq . "$a/$s.body"; then
        diff -u <(normalize "$a/$s.body") <(normalize "$b/$s.body") | sed -n '3,22p' | sed 's/^/      /'
      else
        printf '      バイナリ: %s → %s バイト\n' "$(wc -c < "$a/$s.body" | tr -d ' ')" "$(wc -c < "$b/$s.body" | tr -d ' ')"
      fi
    fi
  done < "$a/index.tsv"
  [ $ng = 0 ] && info "すべて同じでした" || die "違いがあります（意図した変更か確認。保存先 logs/snapshot/$2/）"
}

acquire_lock "snapshot"
case "${1:-snapshot}" in
  snapshot) take "${NAME:-base}" ;;
  compare) compare "${BASE:-base}" "${NAME:-current}" ;;
  *) die "使い方: snapshot.sh snapshot|compare" ;;
esac
