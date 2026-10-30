# docs

積み上げる文書（docs/design・docs/results など）の状態を見て、docs ブランチを main に取り込みます。

## 役割

- `make docs-status` は、results の無い design（ベンチ済みかどうか）と、main に入っていない docs ブランチのコミットを出す。
- `make docs-merge` は、docs ブランチを main に早送りで取り込む。できなければ、docs の worktree（.claude/worktrees/docs）で main を取り込んでから、もう一度早送りする。衝突したら、その取り込みを取り消して止まる。
- 文書を書くのは受け持たない（doc-writer と docs/README.md）。
- hooks（docs/kit/hooks.md）と bench の次の一手は、ここの `docs.sh pending` と `docs.sh unmerged` で判定する。

## いつ使うか

- doc-writer がコミットを返した直後：`make docs-merge`（CLAUDE.md の出来事ごとの手順 6）。
- 状態を知りたいとき：`make docs-status`。

## 動かし方

- scripts/docs.sh の冒頭のコメント、`make help`。

## 確かめ方

- docs-merge の後、`make docs-status` が「docs ブランチはすべて main に入っています」を出す。
- 衝突で止まったら、表示された worktree で手で直してから、もう一度 `make docs-merge`。
- 「main ブランチがありません」と出たら、`git branch -m <今のブランチ> main` で名前を変える（docs.sh・hooks・bench の次の一手は main を前提にする）。

## 更新のしかた

- ブランチ名と worktree の場所：scripts/docs.sh の `DOCS_BRANCH`・`DOCS_WORKTREE`（.claude/agents/doc-writer.md と揃える）。
- 直したら `make lint` と `make test`（tests/test_docs.sh）。
