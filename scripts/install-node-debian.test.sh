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
require "$installer" '12\|13'
require "$installer" '/run/systemd/system'
require "$installer" 'systemctl enable --now vnstat'
require "$installer" 'apt-get install.*nftables|nftables.*procps'
require "$installer" 'ip -o route show default'
require "$installer" 'vnstat --add -i'
require "$installer" 'rollback_activation'
require "$installer" 'mv -f.*\$UNIT'
require "$unit" 'ExecStart=/usr/local/bin/remnanode-lite'
require "$unit" 'AmbientCapabilities=CAP_NET_ADMIN'
require "$unit" 'After=network-online.target vnstat.service'
reject "$installer" 'rc-service|rc-update|/etc/init\.d|apk add|NAT'
reject "$unit" 'openrc|rc-service'

xray_installer="${repo_root}/scripts/install-xray.sh"
require "$xray_installer" 'RNL_WORK_TMPDIR:-/var/tmp'
require "$xray_installer" 'remnanode-xray\.XXXXXX'
require "$xray_installer" "trap 'rm -rf --.*EXIT"

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
grep -q 'systemd 会在重启后自动恢复' "$tmp/dry-run.out"
if grep -q 'OpenRC' "$tmp/dry-run.out"; then
  echo "Debian installer emitted an OpenRC readiness hint" >&2
  exit 1
fi

mkdir -p "$tmp/systemd"
for version in 12 13; do
  printf 'ID=debian\nVERSION_ID=%s\nPRETTY_NAME="Debian GNU/Linux %s"\n' \
    "$version" "$version" >"$tmp/os-release-$version"
  PATH="$tmp/bin:$PATH" \
    RNL_OS_RELEASE_FILE="$tmp/os-release-$version" \
    RNL_SYSTEMD_RUNTIME_DIR="$tmp/systemd" \
    RNL_BINARY_SHA256="$(printf 'a%.0s' {1..64})" \
    bash "$installer" --install --dry-run --yes --skip-xray --port 2443 \
    >"$tmp/debian-$version.out"
done

printf 'ID=debian\nVERSION_ID=11\nPRETTY_NAME="Debian GNU/Linux 11"\n' >"$tmp/os-release-11"
if PATH="$tmp/bin:$PATH" \
  RNL_OS_RELEASE_FILE="$tmp/os-release-11" \
  RNL_SYSTEMD_RUNTIME_DIR="$tmp/systemd" \
  RNL_BINARY_SHA256="$(printf 'a%.0s' {1..64})" \
  bash "$installer" --install --dry-run --yes --skip-xray --port 2443 \
  >"$tmp/debian-11.out" 2>&1; then
  echo "Debian installer accepted unsupported Debian 11" >&2
  exit 1
fi
grep -q 'Debian 12.*13' "$tmp/debian-11.out"

if PATH="$tmp/bin:$PATH" RNL_BINARY_SHA256="$(printf 'a%.0s' {1..64})" \
  bash "$installer" --install --dry-run --yes --skip-xray --port 0 \
  >"$tmp/invalid.out" 2>&1; then
  echo "Debian installer accepted port 0" >&2
  exit 1
fi
grep -q '1-65535' "$tmp/invalid.out"

echo "Debian installer contract passed"
