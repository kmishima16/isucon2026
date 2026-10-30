#!/usr/bin/env bash
# make lint（scripts/lint.sh）の、目次とリンクの確認を確かめる
source "$(dirname "$0")/helpers.sh"

echo "目次とリンク"
make_fixture
write_doc . docs/kit/a.md "# a"
write_doc . docs/flow/f.md "# f"
write_doc . README.md "$(printf '# t\n\n- [a](docs/kit/a.md)\n- [f](docs/flow/f.md)\n- [外](https://example.com)\n- [節](docs/kit/a.md#役割)')"
if bash scripts/lint.sh >/dev/null 2>&1; then ok "目次とリンクが揃っていれば通る"; else ng "目次とリンクが揃っていれば通る"; fi
write_doc . docs/kit/b.md "# b"
if out="$(bash scripts/lint.sh 2>&1)"; then ng "目次に無い手引きを見つける"; else contains "目次に無い手引きを見つける" "docs/kit/b.md" "$out"; fi
rm docs/kit/b.md
printf -- '- [x](../kit/nothing.md)\n' >> docs/flow/f.md
if out="$(bash scripts/lint.sh 2>&1)"; then ng "リンク切れを見つける"; else contains "リンク切れを見つける" "../kit/nothing.md" "$out"; fi
finish_fixture

report
