#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
installer="${repo_root}/scripts/install-node-alpine.sh"
upgrade="${repo_root}/scripts/upgrade.sh"

assert_contains() {
  local file="$1" pattern="$2"
  grep -Eq -- "$pattern" "$file" || {
    echo "expected ${file} to contain: ${pattern}" >&2
    exit 1
  }
}

assert_absent() {
  local pattern="$1"
  shift
  if grep -Eiq -- "$pattern" "$@"; then
    echo "forbidden distribution pattern found: ${pattern}" >&2
    grep -Ein -- "$pattern" "$@" >&2 || true
    exit 1
  fi
}

assert_contains "$installer" 'VERSION="1\.0\.1"'
assert_contains "$installer" '/etc/alpine-release'
assert_contains "$installer" 'RNL_RELEASE_BASE_URL'
assert_contains "$installer" 'SHA256SUMS'
assert_contains "$installer" 'sha256sum -c -'
assert_contains "$installer" 'apk add --no-cache.*nftables.*vnstat|apk add --no-cache.*vnstat.*nftables'
assert_contains "$installer" 'rc-update add vnstat default'
assert_contains "$installer" 'rc-service vnstat (start|restart)'
assert_contains "$installer" 'ip route.*default'
assert_contains "$installer" 'vnstat --add -i'
assert_contains "$installer" 'rc-update add remnawave-node default'

assert_contains "$upgrade" 'VERSION="1\.0\.1"'
assert_contains "$upgrade" 'RNL_RELEASE_BASE_URL'
assert_contains "$upgrade" 'SHA256SUMS'
assert_contains "$upgrade" 'sha256sum -c -'
assert_contains "$upgrade" 'candidate'
assert_contains "$upgrade" 'rollback'
assert_contains "$upgrade" 'wait_for_service_stable'

distribution_files=(
  "${repo_root}/scripts/install-node-alpine.sh"
  "${repo_root}/scripts/install-xray.sh"
  "${repo_root}/scripts/upgrade.sh"
  "${repo_root}/scripts/uninstall.sh"
  "${repo_root}/scripts/install-env-helpers.sh"
  "${repo_root}/deploy/remnawave-node.openrc"
  "${repo_root}/deploy/remnawave-node-run.sh"
  "${repo_root}/cmd/remnanode-lite/main.go"
  "${repo_root}/internal/doctor/doctor.go"
)
assert_absent 'systemd|systemctl|journalctl|apt( |-)install|/etc/debian|install-node\.sh|/main/' "${distribution_files[@]}"

echo "Alpine installer contract passed"
