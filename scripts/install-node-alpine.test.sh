#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
installer="${repo_root}/scripts/install-node-alpine.sh"
installer_en="${repo_root}/scripts/install-node-alpine-en.sh"
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

assert_contains "$installer" 'VERSION="1\.0\.3"'
assert_contains "$installer_en" 'VERSION="1\.0\.3"'
assert_contains "$installer" '/etc/alpine-release'
assert_contains "$installer" 'RNL_RELEASE_BASE_URL'
assert_contains "$installer" 'RNL_INSTALLER_BUNDLE_SHA256'
assert_contains "$installer" 'RNL_BINARY_SHA256_'
assert_contains "$installer" 'sha256sum -c -'
assert_contains "$installer" 'apk add --no-cache.*nftables.*vnstat|apk add --no-cache.*vnstat.*nftables'
assert_contains "$installer" 'rc-update add vnstat default'
assert_contains "$installer" 'rc-service vnstat (start|restart)'
assert_contains "$installer" 'ip route.*default'
assert_contains "$installer" 'vnstat --add -i'
assert_contains "$installer" 'rc-update add remnawave-node default'

assert_contains "$upgrade" 'VERSION="1\.0\.3"'
assert_contains "$upgrade" 'RNL_RELEASE_BASE_URL'
assert_contains "$upgrade" 'RNL_BINARY_SHA256_'
assert_contains "$upgrade" 'sha256sum -c -'
assert_contains "$upgrade" 'candidate'
assert_contains "$upgrade" 'rollback'
assert_contains "$upgrade" 'wait_for_service_stable'

distribution_files=(
  "${repo_root}/scripts/install-node-alpine.sh"
  "${repo_root}/scripts/install-node-alpine-en.sh"
  "${repo_root}/scripts/install-node-alpine.messages.sh"
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

secret_tmp="$(mktemp -d)"
trap 'rm -rf "$secret_tmp"' EXIT
secret_output="$({
  export RNL_LANG=en
  export NODE_ENV="${secret_tmp}/node.env"
  export SECRET_FILE="${secret_tmp}/secret.key"
  export SECRET_KEY="dummy-secret-key"
  export SECRET_FILE_ARG=""
  export YES=1
  export DRY_RUN=0
  export RESTART_CMD="rc-service remnawave-node restart"
  : >"$NODE_ENV"
  # shellcheck source=install-node-alpine.messages.sh
  source "${repo_root}/scripts/install-node-alpine.messages.sh"
  # shellcheck source=install-env-helpers.sh
  source "${repo_root}/scripts/install-env-helpers.sh"
  prompt_secret_key
  SECRET_KEY="replacement-that-must-not-overwrite"
  prompt_secret_key
} 2>&1)"
grep -q '^SECRET_KEY="dummy-secret-key"$' "${secret_tmp}/node.env"
[ "$(grep -c '^SECRET_KEY=' "${secret_tmp}/node.env")" -eq 1 ]
! grep -q 'replacement-that-must-not-overwrite' "${secret_tmp}/node.env"
! grep -Eiq 'paste|粘贴' <<<"$secret_output"
if stat -f '%Lp' "${secret_tmp}/node.env" >/dev/null 2>&1; then
  [ "$(stat -f '%Lp' "${secret_tmp}/node.env")" = "600" ]
else
  [ "$(stat -c '%a' "${secret_tmp}/node.env")" = "600" ]
fi

echo "Alpine installer contract passed"
