#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
launcher="$tmp/install-node.sh"
cp "${repo_root}/scripts/install-node.sh" "$launcher"

mkdir -p "$tmp/safe/remnanode-native-installer/scripts" "$tmp/bin"
cat >"$tmp/safe/remnanode-native-installer/scripts/install-node.sh" <<'EOF'
#!/usr/bin/env bash
echo "verified local bundle executed: $*"
EOF
tar -C "$tmp/safe" -czf "$tmp/safe.tar.gz" remnanode-native-installer

cat >"$tmp/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) output="$2"; shift 2 ;;
    *) shift ;;
  esac
done
cp "${FETCH_FIXTURE:?}" "$output"
EOF
chmod +x "$tmp/bin/curl"

run_launcher() {
  env PATH="$tmp/bin:$PATH" \
    FETCH_FIXTURE="$1" \
    RNL_RELEASE_BASE_URL="https://example.invalid/v1.0.3" \
    RNL_INSTALLER_BUNDLE_SHA256="${2:-}" \
    bash "$launcher" --help
}

if run_launcher "$tmp/safe.tar.gz" "" >"$tmp/missing.out" 2>&1; then
  echo "standalone launcher accepted a missing bundle digest" >&2
  exit 1
fi
grep -q 'requires the reviewed RNL_INSTALLER_BUNDLE_SHA256' "$tmp/missing.out"

if run_launcher "$tmp/safe.tar.gz" "$(printf '0%.0s' {1..64})" >"$tmp/wrong.out" 2>&1; then
  echo "standalone launcher accepted an incorrect bundle digest" >&2
  exit 1
fi
grep -Eqi 'FAILED|mismatch' "$tmp/wrong.out"

safe_sha="$(sha256sum "$tmp/safe.tar.gz" | awk '{print $1}')"
run_launcher "$tmp/safe.tar.gz" "$safe_sha" >"$tmp/safe.out"
grep -q 'verified local bundle executed: --help' "$tmp/safe.out"

mkdir -p "$tmp/unsafe/sub"
touch "$tmp/unsafe/outside"
tar -C "$tmp/unsafe/sub" -czf "$tmp/unsafe.tar.gz" ../outside
unsafe_sha="$(sha256sum "$tmp/unsafe.tar.gz" | awk '{print $1}')"
if run_launcher "$tmp/unsafe.tar.gz" "$unsafe_sha" >"$tmp/unsafe.out" 2>&1; then
  echo "standalone launcher accepted an archive traversal entry" >&2
  exit 1
fi
grep -q 'Unsafe installer bundle entry' "$tmp/unsafe.out"

for file in \
  scripts/install-node.sh \
  scripts/install-node-alpine.sh \
  scripts/install-node-alpine-en.sh \
  scripts/install-xray.sh \
  scripts/upgrade.sh; do
  if grep -Eq 'raw\.githubusercontent\.com|/main/|curl[^|]*\|[[:space:]]*(sudo[[:space:]]+)?bash' "${repo_root}/${file}"; then
    echo "mutable or piped transitive execution found in ${file}" >&2
    exit 1
  fi
done

grep -q 'CUSTOM_CORE_SHA256' "${repo_root}/scripts/install-xray.sh"
grep -q 'ASN_DB_SHA256' "${repo_root}/scripts/install-xray.sh"
grep -q 'XRAY_SHA256_64' "${repo_root}/scripts/install-xray.sh"
grep -q 'RNL_BINARY_SHA256_AMD64' "${repo_root}/.github/workflows/release.yml"
grep -q 'remnanode-native-installer_' "${repo_root}/.github/workflows/release.yml"

echo "installer fetch provenance contract passed"
