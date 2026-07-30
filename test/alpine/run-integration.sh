#!/usr/bin/env bash
set -euo pipefail

workspace="${WORKSPACE:-/workspace}"
http_root="/tmp/release-http"
auth_dir="/tmp/remnaplus-auth"

mkdir -p /run/openrc
touch /run/openrc/softlevel

rm -rf "$http_root"
mkdir -p "$http_root"
cp -R /release/. "$http_root/"
mkdir -p "$http_root/corrupt"
cp "$http_root/ok/"* "$http_root/corrupt/"
printf 'corrupt' >> "$http_root/corrupt/remnanode-lite_linux_amd64.tar.gz"

httpd -f -p 127.0.0.1:18080 -h "$http_root" &
httpd_pid="$!"
diagnostics() {
  local status="$1"
  if [ "$status" -eq 0 ]; then
    return 0
  fi
  echo "---- alpine gate diagnostics ----" >&2
  rc-service remnawave-node status >&2 2>/dev/null || true
  ps aux >&2 || true
  ss -tlnp >&2 2>/dev/null || true
  if [ -f /etc/remnanode/node.env ]; then
    sed -E 's/^(SECRET_KEY=).*/\1[redacted]/' /etc/remnanode/node.env >&2 || true
  fi
  tail -n 80 /var/log/remnanode/openrc.log >&2 2>/dev/null || true
  tail -n 80 /var/log/remnanode/openrc.err.log >&2 2>/dev/null || true
  for file in /tmp/alpine-*.json /tmp/alpine-*.err; do
    [ -f "$file" ] || continue
    echo "---- ${file} ----" >&2
    sed -E 's/(Authorization: Bearer )[A-Za-z0-9._-]+/\1[redacted]/g' "$file" >&2 || true
  done
}
trap 'status=$?; diagnostics "$status"; kill "$httpd_pid" 2>/dev/null || true; exit "$status"' EXIT

ready=0
for _ in $(seq 1 20); do
  if curl -fsS http://127.0.0.1:18080/ok/SHA256SUMS >/dev/null; then
    ready=1
    break
  fi
  sleep 1
done
if [ "$ready" -ne 1 ]; then
  echo "local release HTTP server did not become ready" >&2
  exit 1
fi

if RNL_RELEASE_BASE_URL=http://127.0.0.1:18080/corrupt \
  RNL_INSTALL_XRAY=0 \
  bash "${workspace}/scripts/install-node-alpine.sh" --install --yes --port 2222; then
  echo "checksum corruption install unexpectedly succeeded" >&2
  exit 1
fi

bash "${workspace}/test/alpine/generate-auth-fixture.sh" "$auth_dir"
# shellcheck disable=SC1091
. "${auth_dir}/fixture.env"
export SECRET_KEY TOKEN CA_CERT CLIENT_CERT CLIENT_KEY

SECRET_KEY="$SECRET_KEY" \
  CUSTOM_CORE_URL=http://127.0.0.1:18080/fake-rw-core \
  RNL_OPENRC_DIRECT_START=1 \
  RNL_RELEASE_BASE_URL=http://127.0.0.1:18080/ok \
  bash "${workspace}/scripts/install-node-alpine.sh" --install --yes --port 2222

touch /var/log/remnanode/access.log /var/log/remnanode/error.log
echo '2026/07/30 00:00:00 tcp:127.0.0.1:12345 accepted tcp:example.com:443' >> /var/log/remnanode/access.log

bash "${workspace}/test/alpine/assert-api.sh"

if RNL_RELEASE_BASE_URL=http://127.0.0.1:18080/bad \
  RNL_OPENRC_DIRECT_START=1 \
  bash "${workspace}/scripts/upgrade.sh" --yes; then
  echo "bad candidate upgrade unexpectedly succeeded" >&2
  exit 1
fi

bash "${workspace}/test/alpine/assert-api.sh"
ss -tln | grep -q ':2222 '
vnstat --json d 62 >/tmp/alpine-vnstat.json
grep -q '"interfaces"' /tmp/alpine-vnstat.json
