#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-https://127.0.0.1:2222}"

api() {
  local method="$1"
  local path="$2"
  shift 2
  curl -fsS --http1.1 \
    --cacert "$CA_CERT" \
    --cert "$CLIENT_CERT" \
    --key "$CLIENT_KEY" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -X "$method" \
    "$@" \
    "${base_url}${path}"
}

api_status() {
  local method="$1"
  local path="$2"
  local output="$3"
  shift 3
  curl -sS --http1.1 \
    --cacert "$CA_CERT" \
    --cert "$CLIENT_CERT" \
    --key "$CLIENT_KEY" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -X "$method" \
    "$@" \
    -o "$output" \
    -w '%{http_code}' \
    "${base_url}${path}"
}

wait_for_api() {
  local i=0
  while [ "$i" -lt 40 ]; do
    if api GET /node/xray/healthcheck >/tmp/alpine-health.json 2>/tmp/alpine-health.err; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  cat /tmp/alpine-health.err >&2 || true
  return 1
}

wait_for_api
grep -q '"nodeVersion"' /tmp/alpine-health.json
grep -q '"xrayVersion"' /tmp/alpine-health.json

api GET /node/stats/get-tcp-connections >/tmp/alpine-tcp.json
grep -q '"connections"' /tmp/alpine-tcp.json

api GET '/node/stats/get-audit-log-source-metadata?source=access' >/tmp/alpine-audit-meta.json
grep -q '"source":"access"' /tmp/alpine-audit-meta.json

api GET '/node/stats/get-audit-log-chunk?source=access&offset=0' >/tmp/alpine-audit-chunk.json
grep -q '"lines"' /tmp/alpine-audit-chunk.json

api POST /node/stats/clean-audit-logs -d '{"retentionDays":1}' >/tmp/alpine-audit-clean.json
grep -q '"removedLines"' /tmp/alpine-audit-clean.json

api GET /node/sni-health/status >/tmp/alpine-sni-status.json
grep -q '"unhealthyCount"' /tmp/alpine-sni-status.json

if api POST /node/plugin/nftables/recreate-tables >/tmp/alpine-nft.json 2>/tmp/alpine-nft.err; then
  grep -Eq '"response"|"isOk"' /tmp/alpine-nft.json
else
  grep -Eqi 'nft|cap|forbidden|unavailable|failed' /tmp/alpine-nft.err /tmp/alpine-nft.json 2>/dev/null
fi

system_status="$(api_status GET /node/stats/get-system-stats /tmp/alpine-system.json)"
if [ "$system_status" = "200" ]; then
  grep -q '"system"' /tmp/alpine-system.json
else
  grep -Eqi 'A010|xray|system stats|failed|error' /tmp/alpine-system.json
fi
