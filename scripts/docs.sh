#!/usr/bin/env bash
# 積み上げる文書（docs/design・docs/results）と docs ブランチの状態を見て、main に取り込む。
#   docs.sh pending   : results の無い design を 1 行 1 件で出す（「docs/design/<名前><TAB>benched|not-benched」）
#                       benched は、その design より後にベンチの記録（docs/scores.tsv）があるもの。results を書かせる番になっている
#   docs.sh unmerged  : main に入っていない docs ブランチのコミット数を出す
#   docs.sh status    : 上の 2 つを人間向けに出す（make docs-status）
#   docs.sh merge     : docs ブランチを main に早送りで取り込む（make docs-merge）
# hooks（scripts/hooks/）は pending と unmerged で判定する。手引きは docs/kit/docs.md。
source "$(dirname "$0")/lib.sh"

MAIN_BRANCH=main
DOCS_BRANCH=docs
DOCS_WORKTREE="$ROOT/.claude/worktrees/docs"   # doc-writer が書く場所（.claude/agents/doc-writer.md と揃える）
SCORES=docs/scores.tsv
# main が無いと、ls-tree の失敗が {} の中で消え、pending が何も言わずに空になるので、先に止める
git rev-parse --verify -q "refs/heads/$MAIN_BRANCH" >/dev/null || die "${MAIN_BRANCH} ブランチがありません。git branch -m <今のブランチ> ${MAIN_BRANCH} で名前を変えてください"

has_docs_branch() { git rev-parse --verify -q "refs/heads/$DOCS_BRANCH" >/dev/null; }

# main と docs ブランチの両方にある、そのディレクトリの文書の名前（NNN_<スラッグ>.md）。取り込む前の文書も数えるため
doc_names() {   # ディレクトリ（docs/design など）
  {
    git ls-tree --name-only "$MAIN_BRANCH" -- "$1/"
    if has_docs_branch; then git ls-tree --name-only "$DOCS_BRANCH" -- "$1/"; fi
  } | awk -F/ '$NF ~ /^[0-9][0-9][0-9]_.+\.md$/ { print $NF }' | sort -u
}

# design がコミットされた時刻を、scores.tsv の time 列と同じ形（手元の時刻の YYYYmmdd-HHMMSS）で出す
design_time() {   # 名前
  local ref t
  for ref in "$MAIN_BRANCH" "$DOCS_BRANCH"; do
    if [ "$ref" = "$DOCS_BRANCH" ] && ! has_docs_branch; then continue; fi
    t="$(git log -1 --format=%cd --date=format-local:%Y%m%d-%H%M%S "$ref" -- "docs/design/$1")"
    if [ -n "$t" ]; then echo "$t"; return; fi
  done
}

last_bench_time() {
  if [ -s "$SCORES" ]; then awk -F'\t' 'NR > 1 && $1 != "" { t = $1 } END { print t }' "$SCORES"; fi
}

pending() {
  local results bench name t
  results="$(doc_names docs/results)"
  bench="$(last_bench_time)"
  while IFS= read -r name; do
    [ -n "$name" ] || continue
    if grep -qxF "$name" <<<"$results"; then continue; fi
    t="$(design_time "$name")"
    if [ -n "$bench" ] && [[ "$bench" > "$t" ]]; then
      printf 'docs/design/%s\tbenched\n' "$name"
    else
      printf 'docs/design/%s\tnot-benched\n' "$name"
    fi
  done <<<"$(doc_names docs/design)"
}

unmerged() {
  if has_docs_branch; then git rev-list --count "$MAIN_BRANCH..$DOCS_BRANCH"; else echo 0; fi
}

status() {
  local p n
  p="$(pending)"
  n="$(unmerged)"
  if [ -n "$p" ]; then
    info "results の無い design（benched はベンチ済みで results を書かせる番、not-benched は実装中）:"
    printf '%s\n' "$p" | sed 's/^/  /' >&2
  else
    info "results の無い design はありません"
  fi
  if [ "$n" = 0 ]; then info "docs ブランチはすべて main に入っています"; else warn "main に入っていない docs のコミットが ${n} 件あります → make docs-merge"; fi
}

merge() {
  local cur
  if ! has_docs_branch; then info "docs ブランチがありません（まだ文書を書いていない）"; return; fi
  cur="$(git branch --show-current)"
  [ "$cur" = "$MAIN_BRANCH" ] || die "$MAIN_BRANCH のチェックアウトで実行してください（今は ${cur}）"
  if [ "$(unmerged)" = 0 ]; then info "取り込むものはありません"; return; fi
  if ! git merge --ff-only -q "$DOCS_BRANCH" 2>/dev/null; then
    # main が先に進んでいる（記録のコミットなど）。docs の worktree で main を取り込んでから、もう一度早送りする
    [ -d "$DOCS_WORKTREE" ] || die "早送りできず、docs の worktree（${DOCS_WORKTREE}）もありません"
    if ! git -C "$DOCS_WORKTREE" merge --no-edit -q "$MAIN_BRANCH"; then
      if git -C "$DOCS_WORKTREE" rev-parse -q --verify MERGE_HEAD >/dev/null; then git -C "$DOCS_WORKTREE" merge --abort; fi
      die "docs の worktree で main を取り込むと衝突します。取り込みは取り消しました。${DOCS_WORKTREE} で手で直してから、もう一度 make docs-merge"
    fi
    git merge --ff-only -q "$DOCS_BRANCH" || die "早送りできません（上のメッセージ）"
  fi
  info "取り込みました: $(git log -1 --format='%h %s')"
}

case "${1:-status}" in
  pending) pending ;;
  unmerged) unmerged ;;
  status) status ;;
  merge) merge ;;
  *) die "使い方: docs.sh pending|unmerged|status|merge" ;;
esac
