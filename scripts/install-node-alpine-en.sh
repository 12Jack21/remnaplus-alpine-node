#!/usr/bin/env bash
set -euo pipefail

VERSION="1.0.3"
REPO="${RNL_REPO:-12Jack21/remnaplus-alpine-node}"
TAG="${RNL_TAG:-v${VERSION}}"
RELEASE_BASE="${RNL_RELEASE_BASE_URL:-https://github.com/${REPO}/releases/download/${TAG}}"
SCRIPT_DIR=""
if resolved_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd)"; then
  SCRIPT_DIR="$resolved_script_dir"
fi

export RNL_LANG=en

if [ -n "$SCRIPT_DIR" ] && [ -f "${SCRIPT_DIR}/install-node-alpine.sh" ] && [ -f "${SCRIPT_DIR}/install-node-alpine.messages.sh" ]; then
  exec bash "${SCRIPT_DIR}/install-node-alpine.sh" "$@"
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "Missing command: curl (Alpine: apk add --no-cache curl bash)" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
expected="${RNL_INSTALLER_BUNDLE_SHA256:-}"
if ! [[ "$expected" =~ ^[A-Fa-f0-9]{64}$ ]]; then
  echo "Standalone installation requires the reviewed RNL_INSTALLER_BUNDLE_SHA256." >&2
  exit 1
fi
bundle="remnanode-native-installer_${TAG}.tar.gz"
curl -fsSL "${RELEASE_BASE}/${bundle}" -o "${tmp_dir}/${bundle}"
printf '%s  %s\n' "${expected,,}" "${tmp_dir}/${bundle}" | sha256sum -c -
while IFS= read -r entry; do
  case "$entry" in
    /*|../*|*/../*|*/..) echo "Unsafe installer bundle entry: $entry" >&2; exit 1 ;;
  esac
done < <(tar -tzf "${tmp_dir}/${bundle}")
tar --no-same-owner --no-same-permissions -xzf "${tmp_dir}/${bundle}" -C "$tmp_dir"
exec env RNL_LANG=en bash "${tmp_dir}/remnanode-native-installer/scripts/install-node-alpine.sh" "$@"
