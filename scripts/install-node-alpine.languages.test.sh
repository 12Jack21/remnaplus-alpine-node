#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
zh_installer="${script_dir}/install-node-alpine.sh"
en_installer="${script_dir}/install-node-alpine-en.sh"
messages="${script_dir}/install-node-alpine.messages.sh"

bash -n "$zh_installer" "$en_installer" "$messages"

zh_version="$(sed -n 's/^VERSION="\([^"]*\)"/\1/p' "$zh_installer" | head -n1)"
en_version="$(sed -n 's/^VERSION="\([^"]*\)"/\1/p' "$en_installer" | head -n1)"
[ -n "$zh_version" ]
[ "$zh_version" = "$en_version" ]

zh_help="$(RNL_LANG=zh bash "$zh_installer" --help)"
en_help="$(RNL_LANG=en bash "$en_installer" --help)"

for flag in --install --upgrade --uninstall --yes --dry-run --skip-xray --low-memory --port --secret-file; do
  grep -q -- "$flag" <<<"$zh_help"
  grep -q -- "$flag" <<<"$en_help"
done

for text in '安装' '端口必须' 'Secret Key' 'vnStat' 'OpenRC' '安装完成' '安装失败'; do
  grep -q -- "$text" <<<"$zh_help"
done
for text in 'Install' 'Port must' 'Secret Key' 'vnStat' 'OpenRC' 'Installation complete' 'Installation failed'; do
  grep -q -- "$text" <<<"$en_help"
done

if RNL_LANG=fr bash "$zh_installer" --help >/tmp/remnaplus-alpine-lang.out 2>&1; then
  echo "unknown installer language unexpectedly succeeded" >&2
  exit 1
fi
grep -q 'Unsupported installer language' /tmp/remnaplus-alpine-lang.out
rm -f /tmp/remnaplus-alpine-lang.out

echo "Alpine installer language contract passed"
