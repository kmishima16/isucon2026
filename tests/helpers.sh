#!/usr/bin/env bash
# テストの共通部品。各テストは、本物の scripts/ を写した使い捨てのリポジトリで動かす（本物のリポジトリと docs ブランチを汚さないため）
set -euo pipefail
REPO_SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fails=0

ok() { printf '  ok  %s\n' "$1"; }
ng() { printf '  NG  %s\n' "$1"; fails=$((fails + 1)); }
check() {   # 説明 期待 実際
  if [ "$2" = "$3" ]; then ok "$1"; else ng "$1"; printf '      期待: %s\n      実際: %s\n' "$2" "$3"; fi
}
contains() {   # 説明 含むはずの文字列 実際
  case "$3" in *"$2"*) ok "$1" ;; *) ng "$1"; printf '      含むはず: %s\n      実際: %s\n' "$2" "$3" ;; esac
}

# 使い捨てのリポジトリを作って移動する。scripts/・isu.env・Makefile と、見出しだけの docs/scores.tsv を入れる
make_fixture() {
  FIX="$(mktemp -d)"
  cp -R "$REPO_SRC/scripts" "$REPO_SRC/isu.env" "$REPO_SRC/Makefile" "$FIX/"
  mkdir -p "$FIX/docs"
  printf 'time\trev\tpass\tscore\tmemo\texpect\n' > "$FIX/docs/scores.tsv"
  printf '.claude/worktrees/\nlogs/\n.lock/\n' > "$FIX/.gitignore"
  cd "$FIX"
  git init -q -b main
  git config user.email test@example.com
  git config user.name test
  git add -A
  git commit -q -m init
}
finish_fixture() { cd "$REPO_SRC"; rm -rf "$FIX"; }

write_doc() {   # ルート（リポジトリか worktree） パス 中身
  mkdir -p "$1/$(dirname "$2")"
  printf '%s\n' "$3" > "$1/$2"
}
# 時刻を決めてコミットする（design と記録の時刻の前後を作るため）
commit_at() {   # ディレクトリ 時刻（YYYY-mm-dd HH:MM:SS、手元の時刻） メッセージ
  (cd "$1" && GIT_AUTHOR_DATE="$2" GIT_COMMITTER_DATE="$2" git commit -q -m "$3")
}

report() {
  if [ "$fails" = 0 ]; then echo "  すべて通りました"; else echo "  NG が ${fails} 件あります"; exit 1; fi
}
