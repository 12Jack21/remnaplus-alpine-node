#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_dir=""
manifest=""
tag=""

die() {
  printf 'build-release-assets: %s\n' "$*" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --output) output_dir="${2:-}"; shift 2 ;;
    --manifest) manifest="${2:-}"; shift 2 ;;
    --tag) tag="${2:-}"; shift 2 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[ -n "$output_dir" ] || die "--output is required"
[ -n "$manifest" ] || die "--manifest is required"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "--tag must be immutable SemVer"
[ ! -e "$output_dir" ] || die "output path already exists: $output_dir"
[ ! -e "$manifest" ] || die "manifest path already exists: $manifest"

git_root="$(git -C "$repo_root" rev-parse --show-toplevel)"
repo_prefix="$(git -C "$repo_root" rev-parse --show-prefix)"
case "$repo_prefix" in
  "") treeish='HEAD^{tree}'; source_commit="$(git -C "$repo_root" rev-parse HEAD)"; diff_scope='.' ;;
  "node-go/") treeish='HEAD:node-go'; source_commit="$(git -C "$git_root" subtree split --prefix=node-go HEAD)"; diff_scope='node-go' ;;
  *) die "node-go must be a repository root or the node-go/ subtree" ;;
esac

if [ -n "$(git -C "$git_root" status --porcelain -- "$diff_scope")" ]; then
  die "node-go source is dirty; commit it before building candidate artifacts"
fi

version_value="$(sed -n 's/.*Version = "\([^"]*\)".*/\1/p' "$repo_root/internal/version/version.go" | head -n1)"
[ "$tag" = "v${version_value}" ] || die "tag $tag does not match Go version $version_value"
contract_version="$(tr -d ' \n\r' < "$repo_root/internal/version/contract.version")"
node -e '
const fs = require("fs")
const value = JSON.parse(fs.readFileSync(process.argv[1], "utf8"))
if (value.contractVersion !== process.argv[2] || !Array.isArray(value.capabilities)) process.exit(1)
' "$repo_root/internal/version/release-capabilities.json" "$contract_version" || \
  die "release capability descriptor is invalid"
source_tree="$(git -C "$git_root" rev-parse "$treeish")"
source_epoch="$(git -C "$git_root" show -s --format=%ct HEAD)"

work_dir="$(mktemp -d)"
trap 'rm -rf -- "$work_dir"' EXIT
mkdir -p "$output_dir"

archive_dir() {
  go run "$repo_root/scripts/release-archive.go" \
    --root "$1" --output "$2" --prefix "${3:-}" --epoch "$source_epoch"
}

for arch in amd64 arm64; do
  stage="$work_dir/binary-$arch"
  mkdir -p "$stage"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -trimpath \
      -ldflags="-s -w -X github.com/12Jack21/remnaplus-alpine-node/internal/version.Version=${version_value} -X github.com/12Jack21/remnaplus-alpine-node/internal/version.ContractVersion=${contract_version}" \
      -o "$stage/remnanode-lite" "$repo_root/cmd/remnanode-lite"
  printf '%s\n\n%s\n' \
    "remnanode-lite ${tag} linux/${arch}" \
    'Extract and install to /usr/local/bin/remnanode-lite' > "$stage/README.txt"
  archive_dir "$stage" "$output_dir/remnanode-lite_linux_${arch}.tar.gz"
done

amd64_sha="$(sha256sum "$output_dir/remnanode-lite_linux_amd64.tar.gz" | awk '{print $1}')"
arm64_sha="$(sha256sum "$output_dir/remnanode-lite_linux_arm64.tar.gz" | awk '{print $1}')"
installer_root="$work_dir/installer/remnanode-native-installer"
mkdir -p "$installer_root/scripts" "$installer_root/deploy"
cp \
  "$repo_root/scripts/install-node.sh" \
  "$repo_root/scripts/install-node-alpine.sh" \
  "$repo_root/scripts/install-node-alpine-en.sh" \
  "$repo_root/scripts/install-node-alpine.messages.sh" \
  "$repo_root/scripts/install-env-helpers.sh" \
  "$repo_root/scripts/install-xray.sh" \
  "$repo_root/scripts/upgrade.sh" \
  "$repo_root/scripts/uninstall.sh" \
  "$installer_root/scripts/"
cp \
  "$repo_root/deploy/remnawave-node.service" \
  "$repo_root/deploy/remnawave-node.openrc" \
  "$repo_root/deploy/remnawave-node-run.sh" \
  "$installer_root/deploy/"
printf 'RNL_BINARY_SHA256_AMD64=%q\nRNL_BINARY_SHA256_ARM64=%q\n' \
  "$amd64_sha" "$arm64_sha" > "$installer_root/release.env"
chmod 0644 "$installer_root/release.env"
archive_dir "$work_dir/installer" "$output_dir/remnanode-native-installer_${tag}.tar.gz"

source_stage="$work_dir/source"
mkdir -p "$source_stage"
git -C "$git_root" archive "$treeish" | tar -xf - -C "$source_stage"
archive_dir "$source_stage" "$output_dir/remnaplus-alpine-node-source-${tag}.tar.gz" "remnaplus-alpine-node-${tag}"

(
  cd "$output_dir"
  sha256sum \
    remnanode-lite_linux_amd64.tar.gz \
    remnanode-lite_linux_arm64.tar.gz \
    "remnanode-native-installer_${tag}.tar.gz" \
    "remnaplus-alpine-node-source-${tag}.tar.gz" > SHA256SUMS
)

installer_sha="$(sha256sum "$output_dir/remnanode-native-installer_${tag}.tar.gz" | awk '{print $1}')"
source_sha="$(sha256sum "$output_dir/remnaplus-alpine-node-source-${tag}.tar.gz" | awk '{print $1}')"
mkdir -p "$(dirname "$manifest")"
cat > "$manifest" <<EOF
{
  "schemaVersion": 1,
  "repository": "12Jack21/remnaplus-alpine-node",
  "tag": "${tag}",
  "sourceCommit": "${source_commit}",
  "sourceTree": "${source_tree}",
  "contractVersion": "${contract_version}",
  "capabilities": $(node -e 'const v=require(process.argv[1]); process.stdout.write(JSON.stringify(v.capabilities))' "$repo_root/internal/version/release-capabilities.json"),
  "assets": {
    "remnanode-lite_linux_amd64.tar.gz": "${amd64_sha}",
    "remnanode-lite_linux_arm64.tar.gz": "${arm64_sha}",
    "remnanode-native-installer_${tag}.tar.gz": "${installer_sha}",
    "remnaplus-alpine-node-source-${tag}.tar.gz": "${source_sha}"
  }
}
EOF

printf 'built %s from %s\nmanifest: %s\n' "$tag" "$source_commit" "$manifest"
