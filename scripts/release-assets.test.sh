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
require 'CGO_ENABLED=0 GOOS=linux GOARCH="?\$\{arch\}"?'
require 'remnanode-lite_linux_amd64\.tar\.gz'
require 'remnanode-lite_linux_arm64\.tar\.gz'
require 'remnanode-native-installer_\$\{tag\}\.tar\.gz'
require 'scripts/install-node\.sh'
require 'deploy/remnawave-node\.service'
require 'RNL_BINARY_SHA256_AMD64'
require 'RNL_BINARY_SHA256_ARM64'
require 'remnaplus-alpine-node-source-\$\{[^}]+\}\.tar\.gz'
require 'git archive'
require 'sha256sum.*SHA256SUMS'
require 'dist/SHA256SUMS'
require 'dist/remnaplus-alpine-node-source-'

echo "release asset contract passed"
