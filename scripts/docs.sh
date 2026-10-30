#!/usr/bin/env bash
# 積み上げる文書（doc-writer が書く overview・retro）の docs ブランチの状態を見て、main に取り込む。
#   docs.sh unmerged  : main に入っていない docs ブランチのコミット数を出す
#   docs.sh status    : 上を人間向けに出す（make docs-status）
#   docs.sh merge     : docs ブランチを main に早送りで取り込む（make docs-merge）
# hooks（scripts/hooks/）は unmerged で判定する。手引きは docs/kit/docs.md。
source "$(dirname "$0")/lib.sh"

MAIN_BRANCH=main
DOCS_BRANCH=docs
DOCS_WORKTREE="$ROOT/.claude/worktrees/docs"   # doc-writer が書く場所（.claude/agents/doc-writer.md と揃える）
# main が無いと、rev-list の失敗で unmerged が何も言わずに空になるので、先に止める
git rev-parse --verify -q "refs/heads/$MAIN_BRANCH" >/dev/null || die "${MAIN_BRANCH} ブランチがありません。git branch -m <今のブランチ> ${MAIN_BRANCH} で名前を変えてください"

has_docs_branch() { git rev-parse --verify -q "refs/heads/$DOCS_BRANCH" >/dev/null; }

unmerged() {
  if has_docs_branch; then git rev-list --count "$MAIN_BRANCH..$DOCS_BRANCH"; else echo 0; fi
}

status() {
  local n
  n="$(unmerged)"
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
  unmerged) unmerged ;;
  status) status ;;
  merge) merge ;;
  *) die "使い方: docs.sh unmerged|status|merge" ;;
esac
