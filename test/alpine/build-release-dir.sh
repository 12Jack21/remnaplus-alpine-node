#!/usr/bin/env bash
set -euo pipefail

out_dir="${1:?usage: build-release-dir.sh <release-dir>}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

rm -rf "$out_dir"
mkdir -p "${out_dir}/ok" "${out_dir}/bad"

version_value="$(sed -n 's/.*Version = "\([^"]*\)".*/\1/p' "${repo_root}/internal/version/version.go" | head -n1)"
contract_version="$(tr -d ' \n\r' < "${repo_root}/internal/version/contract.version")"

build_archive() {
  local arch="$1"
  local dest="$2"
  local work
  work="$(mktemp -d)"

  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -trimpath \
      -ldflags="-s -w -X github.com/12Jack21/remnaplus-alpine-node/internal/version.Version=${version_value} -X github.com/12Jack21/remnaplus-alpine-node/internal/version.ContractVersion=${contract_version}" \
      -o "${work}/remnanode-lite" \
      "${repo_root}/cmd/remnanode-lite"

  printf '%s\n' "remnaplus-alpine-node v${version_value} linux/${arch}" > "${work}/README.txt"
  tar -C "$work" -czf "${dest}/remnanode-lite_linux_${arch}.tar.gz" remnanode-lite README.txt
  rm -rf "$work"
}

build_archive amd64 "${out_dir}/ok"
build_archive arm64 "${out_dir}/ok"
(cd "${out_dir}/ok" && sha256sum remnanode-lite_linux_amd64.tar.gz remnanode-lite_linux_arm64.tar.gz > SHA256SUMS)

tmp_bad="$(mktemp -d)"
cat > "${tmp_bad}/bad-main.go" <<'GO'
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println("remnaplus-alpine-node 9.9.9 contract 2.8.0")
		return
	}
	os.Exit(42)
}
GO
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "${tmp_bad}/remnanode-lite" "${tmp_bad}/bad-main.go"
printf '%s\n' "intentionally failing candidate for rollback test" > "${tmp_bad}/README.txt"
tar -C "$tmp_bad" -czf "${out_dir}/bad/remnanode-lite_linux_amd64.tar.gz" remnanode-lite README.txt
(cd "${out_dir}/bad" && sha256sum remnanode-lite_linux_amd64.tar.gz > SHA256SUMS)
rm -rf "$tmp_bad"

cat > "${out_dir}/fake-rw-core" <<'SH'
#!/bin/sh
if [ "${1:-}" = "version" ]; then
  echo "Xray 26.6.27"
  exit 0
fi
if [ "${1:-}" = "tls" ]; then
  echo "fake rw-core does not perform TLS probes" >&2
  exit 1
fi
echo "fake rw-core only supports version in packaged Alpine gate" >&2
exit 1
SH
chmod 0755 "${out_dir}/fake-rw-core"
