# GitHub Release Checklist

This public repository is published from the private RemnaPlus monorepo by
subtree split. Do not publish directly from a dirty or hand-edited checkout.

## Version Alignment

Before publishing, these values must agree:

| File | Field |
|------|-------|
| `internal/version/version.go` | `var Version` |
| `internal/version/contract.version` | RemnaPlus node contract |
| `scripts/install-node.sh` | `VERSION=` |
| `scripts/install-node-alpine.sh` | `VERSION=` |
| `scripts/install-node-alpine-en.sh` | `VERSION=` |
| `scripts/upgrade.sh` | `VERSION=` |
| `scripts/uninstall.sh` | `VERSION=` |
| `internal/version/release-capabilities.json` | reviewed native capabilities |

Current public release: `v1.0.3`. Source version `v1.0.4` is only a candidate
until both real-host canaries and published-byte verification pass. The
dashboard recommendation remains on `v1.0.3` during that process.

## Local Gates

```bash
go test ./...
for script in scripts/*.sh deploy/remnawave-node-run.sh deploy/remnawave-node.openrc; do bash -n "$script"; done
bash scripts/install-node-debian.test.sh
bash scripts/install-node-alpine.test.sh
bash scripts/install-node-alpine.languages.test.sh
bash scripts/release-assets.test.sh
```

The monorepo publisher also runs the packaged Alpine gate before it pushes a
public tag.

## Release Assets

Pushing an immutable tag builds and uploads:

- `remnanode-lite_linux_amd64.tar.gz`
- `remnanode-lite_linux_arm64.tar.gz`
- `remnanode-native-installer_${tag}.tar.gz`
- `remnaplus-alpine-node-source-${tag}.tar.gz`
- `SHA256SUMS`

Operators install only from pinned tags. The installer downloads the selected
archive and `SHA256SUMS` from the same release and verifies the archive before
extracting it.

The installer bundle contains both Alpine/OpenRC and Debian/systemd service
assets plus the complete pinned helper set. The monorepo builds deterministic
candidate bytes first, canaries those exact bytes on both init systems, then
rejects a public release whose commit or checksums differ.

## Public Alpine Verification

On an Alpine/OpenRC VPS:

```bash
curl -fsSL 'https://raw.githubusercontent.com/12Jack21/remnaplus-alpine-node/v1.0.4/scripts/install-node-alpine.sh' \
  -o '/tmp/remnaplus-alpine-node.sh' &&
read -rsp 'Node Secret Key: ' NODE_SECRET; echo
umask 077; printf '%s' "$NODE_SECRET" > /tmp/remnanode-secret.key; unset NODE_SECRET
env RNL_TAG='v1.0.4' bash '/tmp/remnaplus-alpine-node.sh' \
  --install --yes --port 2222 --secret-file /tmp/remnanode-secret.key
rm -f /tmp/remnanode-secret.key
rc-service remnawave-node status
remnanode-lite doctor
tail -n 50 /var/log/remnanode/openrc.log
```

## Rollback

The upgrade script is transactional. If the candidate binary fails to become
stable, it restores the previous binary and restarts the OpenRC service.

Manual restore from a retained backup:

```bash
rc-service remnawave-node stop
cp /usr/local/bin/remnanode-lite.bak.TIMESTAMP /usr/local/bin/remnanode-lite
setcap cap_net_admin+ep /usr/local/bin/remnanode-lite
rc-service remnawave-node start
```

To downgrade Alpine to the previous verified native release:

```bash
RNL_TAG=v1.0.3 bash scripts/upgrade.sh --yes
```

The first Debian-native release has no compatible prior Debian-native tag.
Retain the previous deployment and configuration as its rollback; do not treat
stock upstream as interchangeable.
