#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workflow="${repo_root}/.github/workflows/release.yml"

require() {
  local pattern="$1"
  grep -Eq -- "$pattern" "$workflow" || {
    echo "release workflow is missing: ${pattern}" >&2
    exit 1
  }
}

require 'go test ./\.\.\.'
require 'scripts/build-release-assets\.sh'
require 'remnanode-lite_linux_amd64\.tar\.gz'
require 'remnanode-lite_linux_arm64\.tar\.gz'
require 'remnanode-native-installer_\$\{\{ github\.ref_name \}\}\.tar\.gz'
require 'remnaplus-alpine-node-source-\$\{\{ github\.ref_name \}\}\.tar\.gz'
require 'dist/SHA256SUMS'
require 'dist/remnaplus-alpine-node-source-'

builder="${repo_root}/scripts/build-release-assets.sh"
archiver="${repo_root}/scripts/release-archive.go"
test -x "$builder" || { echo "release asset builder is not executable" >&2; exit 1; }
test -f "$archiver" || { echo "deterministic release archiver is missing" >&2; exit 1; }
grep -q 'os.O_EXCL' "$archiver"
grep -q 'sourceCommit' "$builder"
grep -q -- '-buildvcs=false' "$builder"
grep -q 'scripts/install-node.sh' "$builder"
grep -q 'remnawave-node.service' "$builder"
grep -q 'remnawave-node.openrc' "$builder"
grep -q 'RNL_BINARY_SHA256_AMD64' "$builder"
grep -q 'RNL_BINARY_SHA256_ARM64' "$builder"
grep -q 'git .* archive' "$builder"
grep -q 'SHA256SUMS' "$builder"

integration_harness="${repo_root}/test/alpine/run-integration.sh"
grep -q 'candidate_checksum amd64' "$integration_harness"
grep -q 'candidate_checksum arm64' "$integration_harness"
grep -q 'export RNL_BINARY_SHA256_AMD64 RNL_BINARY_SHA256_ARM64' "$integration_harness"
grep -q 'export CUSTOM_CORE_SHA256' "$integration_harness"

tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
mkdir -p "$tmp/existing"
printf 'keep\n' > "$tmp/existing/marker"
if bash "${repo_root}/test/alpine/build-release-dir.sh" "$tmp/existing" >/dev/null 2>&1; then
  echo "fixture builder accepted an existing output directory" >&2
  exit 1
fi
test -f "$tmp/existing/marker"
if bash "$builder" --tag v1.0.4 --output "$tmp/existing" --manifest "$tmp/manifest.json" >/dev/null 2>&1; then
  echo "production builder accepted an existing output directory" >&2
  exit 1
fi
test -f "$tmp/existing/marker"

echo "release asset contract passed"
