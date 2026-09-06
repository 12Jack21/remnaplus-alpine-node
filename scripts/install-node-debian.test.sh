#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
installer="${repo_root}/scripts/install-node.sh"
unit="${repo_root}/deploy/remnawave-node.service"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT

require() {
  local file="$1" pattern="$2"
  grep -Eq -- "$pattern" "$file" || {
    echo "expected ${file} to contain: ${pattern}" >&2
    exit 1
  }
}

reject() {
  local file="$1" pattern="$2"
  if grep -Eq -- "$pattern" "$file"; then
    echo "forbidden Debian installer pattern found: ${pattern}" >&2
    exit 1
  fi
}

require "$installer" '/etc/os-release'
require "$installer" 'ID:-.*debian'
require "$installer" '/run/systemd/system'
require "$installer" 'systemctl enable --now vnstat'
require "$installer" 'ip -o route show default'
require "$installer" 'vnstat --add -i'
require "$installer" 'rollback_activation'
require "$installer" 'mv -f.*\$UNIT'
require "$unit" 'ExecStart=/usr/local/bin/remnanode-lite'
require "$unit" 'AmbientCapabilities=CAP_NET_ADMIN'
require "$unit" 'After=network-online.target vnstat.service'
reject "$installer" 'rc-service|rc-update|/etc/init\.d|apk add|NAT'
reject "$unit" 'openrc|rc-service'

mkdir -p "$tmp/bin"
for command_name in apt-get systemctl vnstat ss ip; do
  command_path="$tmp/bin/$command_name"
  {
    echo '#!/usr/bin/env bash'
    echo 'exit 0'
  } >"$command_path"
  chmod +x "$command_path"
done

PATH="$tmp/bin:$PATH" \
  RNL_BINARY_SHA256="$(printf 'a%.0s' {1..64})" \
  bash "$installer" --install --dry-run --yes --skip-xray --port 2443 \
  >"$tmp/dry-run.out"
grep -q '监听端口：2443' "$tmp/dry-run.out"
grep -q 'systemctl enable --now vnstat' "$tmp/dry-run.out"

if PATH="$tmp/bin:$PATH" RNL_BINARY_SHA256="$(printf 'a%.0s' {1..64})" \
  bash "$installer" --install --dry-run --yes --skip-xray --port 0 \
  >"$tmp/invalid.out" 2>&1; then
  echo "Debian installer accepted port 0" >&2
  exit 1
fi
grep -q '1-65535' "$tmp/invalid.out"

echo "Debian installer contract passed"
