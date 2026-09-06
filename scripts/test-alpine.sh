#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
harness_dir="${repo_root}/test/alpine"

required_files=(
  Dockerfile
  build-release-dir.sh
  generate-auth-fixture.sh
  run-integration.sh
  assert-api.sh
)

for file in "${required_files[@]}"; do
  if [ ! -f "${harness_dir}/${file}" ]; then
    echo "missing Alpine integration harness file: test/alpine/${file}" >&2
    exit 1
  fi
done

if grep -R "cp .*${repo_root}.*/usr/local/bin/remnanode-lite" "${harness_dir}" >/dev/null 2>&1; then
  echo "harness must install from release artifacts, not copy a developer binary into place" >&2
  exit 1
fi

work_dir="${TMPDIR:-/tmp}/remnaplus-alpine-node-test"
release_dir="${work_dir}/release"

rm -rf "$work_dir"
mkdir -p "$work_dir"

bash "${harness_dir}/build-release-dir.sh" "$release_dir"

docker build -t remnaplus-alpine-node-gate "${harness_dir}"
docker run --rm --privileged \
  -v "${repo_root}:/workspace:ro" \
  -v "${release_dir}:/release:ro" \
  remnaplus-alpine-node-gate \
  /workspace/test/alpine/run-integration.sh
