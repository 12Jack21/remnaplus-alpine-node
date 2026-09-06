#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# The provenance test parses upstream.lock; the remaining tests exercise local contracts.
go test ./internal/artifact ./internal/xray ./internal/contract ./internal/auditlog ./internal/snihealth ./internal/tcpstats \
  ./internal/vnstat ./internal/stats ./internal/secret ./internal/httpserver \
  ./internal/version ./internal/instance ./cmd/remnanode-lite
bash scripts/dashboard-secret-compat.test.sh
if [ -n "${RNL_OFFICIAL_NODE_CHECKOUT:-}" ]; then
  python3 scripts/verify-contract-sync.py "$RNL_OFFICIAL_NODE_CHECKOUT"
fi
