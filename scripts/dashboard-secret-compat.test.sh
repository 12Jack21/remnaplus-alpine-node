#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="${ROOT_DIR}/backend"
umask 077
FIXTURE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/remnaplus-secret-compat.XXXXXX")"
FIXTURE_PATH="${FIXTURE_DIR}/secret.fixture"
cleanup() {
  rm -f -- "$FIXTURE_PATH"
  rmdir -- "$FIXTURE_DIR"
}
trap cleanup EXIT

if [ ! -f "${BACKEND_DIR}/node_modules/tsx/dist/cli.mjs" ]; then
  echo "backend dependencies are required: cd backend && npm install" >&2
  exit 1
fi

node "${BACKEND_DIR}/node_modules/tsx/dist/cli.mjs" \
  "${ROOT_DIR}/node-go/scripts/generate-dashboard-secret-fixture.ts" "$FIXTURE_PATH"

cd "${ROOT_DIR}/node-go"
RNL_DASHBOARD_SECRET_FIXTURE="$FIXTURE_PATH" \
  go test ./internal/secret -run '^TestDashboardGeneratedSecret$' -count=1
