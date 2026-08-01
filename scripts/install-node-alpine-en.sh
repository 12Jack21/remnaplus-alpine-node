#!/usr/bin/env bash
set -euo pipefail

VERSION="1.0.1"
REPO="${RNL_REPO:-12Jack21/remnaplus-alpine-node}"
TAG="${RNL_TAG:-v${VERSION}}"
RAW_BASE="${RNL_RAW_BASE_URL:-https://raw.githubusercontent.com/${REPO}/${TAG}}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || true)"

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
for file in install-node-alpine.sh install-node-alpine.messages.sh install-env-helpers.sh install-xray.sh; do
  curl -fsSL "${RAW_BASE}/scripts/${file}" -o "${tmp_dir}/${file}"
done
bash "${tmp_dir}/install-node-alpine.sh" "$@"
