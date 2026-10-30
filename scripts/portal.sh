#!/usr/bin/env bash
# ポータルのベンチ結果を取り出して表示し、まだ記録していなければ docs/scores.tsv に記録する（本番用の骨組み）。
#   make portal [E=期待した動き]          : いちばん新しい結果を 1 回取り出す
#   make portal WATCH=1 [E=期待した動き]  : 新しい結果が出るまで 15 秒ごとに見に行く（ポータルでベンチを始めた直後に）
# 取り出し方は isu.env の PORTAL_RESULT_URL（Cookie 付きで GET）か PORTAL_RESULT_CMD（JSON を出すコマンド）。
# 結果の JSON のどこに何があるかは PORTAL_JQ_* で指定する。本番当日にポータルを開発者ツールで見て埋める。
# 認証の Cookie は .envrc の PORTAL_COOKIE に書く（git に入れない。Claude はこの値を読まない・書かない）。
source "$(dirname "$0")/lib.sh"
refuse_in_worktree portal
command -v jq >/dev/null || die "jq がありません"
SEEN="$LOG_DIR/portal-seen"; touch "$SEEN"

fetch() {
  if [ -n "${PORTAL_RESULT_URL:-}" ]; then
    [ -n "${PORTAL_COOKIE:-}" ] || die "PORTAL_COOKIE が空です（.envrc に書いて direnv allow）"
    curl -fsS --max-time 20 -H "Cookie: $PORTAL_COOKIE" -H 'Accept: application/json' "$PORTAL_RESULT_URL"
  elif [ -n "${PORTAL_RESULT_CMD:-}" ]; then
    bash -c "$PORTAL_RESULT_CMD"
  else
    die "isu.env の PORTAL_RESULT_URL か PORTAL_RESULT_CMD を設定してください"
  fi
}

# 1 件の結果を取り出す。新しいものがあれば 0、記録済みなら 1
check() {
  local json id pass score
  json="$(fetch)" || die "結果を取り出せません"
  # 結果が配列で返るポータルでは、PORTAL_JQ_PICK で 1 件を選ぶ（例: '.[0]'）
  json="$(jq -c "$PORTAL_JQ_PICK" <<<"$json")" || die "JSON として読めません: $(head -c 200 <<<"$json")"
  id="$(jq -r "${PORTAL_JQ_ID:-empty}" <<<"$json")"
  [ -n "$id" ] && [ "$id" != null ] || id="sha:$(shasum <<<"$json" | cut -c1-12)"   # ID の無い結果は中身で見分ける
  if grep -qxF "$id" "$SEEN"; then return 1; fi
  pass="$(jq -r "$PORTAL_JQ_PASS" <<<"$json")"
  score="$(jq -r "$PORTAL_JQ_SCORE" <<<"$json")"
  T="$(ts)"; printf '%s\n' "$json" > "$LOG_DIR/$T-portal.json"
  info "ポータルの結果 $id: pass=$pass score=${score}（logs/$T-portal.json）"
  jq -r "$PORTAL_JQ_MESSAGES // [] | .[]?" <<<"$json" | head -n 10 | sed 's/^/  /'
  [[ "$score" =~ ^-?[0-9]+$ ]] || die "スコアを数値として読めません: ${score}（PORTAL_JQ_SCORE を確認）"
  # check は && や || の中から呼ぶので set -e が効かない。記録に失敗したら、ここで止めて ID を記録済みにしない
  PASS="$pass" S="$score" M="${M:-portal $id}" bash "$ROOT/scripts/bench.sh" score || die "記録できませんでした（上のメッセージ）"
  echo "$id" >> "$SEEN"
}

if [ "${WATCH:-0}" = 1 ]; then
  info "新しい結果を待っています（15 秒ごと、最長 10 分）"
  for _ in $(seq 1 40); do check && exit 0; sleep 15; done
  die "10 分たっても新しい結果が出ませんでした"
fi
check || info "新しい結果はありません（いちばん新しいものは記録済み）"
