# docs

積み上げる文書（doc-writer が書く overview・retro）の docs ブランチの状態を見て、main に取り込みます。

## 役割

- `make docs-status` は、main に入っていない docs ブランチのコミットを出す。
- `make docs-merge` は、docs ブランチを main に早送りで取り込む。できなければ、docs の worktree（.claude/worktrees/docs）で main を取り込んでから、もう一度早送りする。衝突したら、その取り込みを取り消して止まる。
- 文書を書くのは受け持たない（doc-writer と docs/README.md）。doc-writer を使うのは、overview（仕様メモ）と retro だけにする。施策ごとの design と results は、今は新しく書かない。
- hooks（docs/kit/hooks.md）は、ここの `docs.sh unmerged` で判定する。

## いつ使うか

- doc-writer がコミットを返した直後：操作役が `make docs-merge`。
- 状態を知りたいとき：`make docs-status`。

## 動かし方

- scripts/docs.sh の冒頭のコメント、`make help`。

## 確かめ方

- docs-merge の後、`make docs-status` が「docs ブランチはすべて main に入っています」を出す。
- 衝突で止まったら、表示された worktree で手で直してから、もう一度 `make docs-merge`。
- 「main ブランチがありません」と出たら、`git branch -m <今のブランチ> main` で名前を変える（docs.sh と hooks は main を前提にする）。

## 更新のしかた

- ブランチ名と worktree の場所：scripts/docs.sh の `DOCS_BRANCH`・`DOCS_WORKTREE`（.claude/agents/doc-writer.md と揃える）。
- 直したら `make lint` と `make test`（tests/test_docs.sh）。
