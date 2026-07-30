# GitHub Release Checklist

This public repository is published from the private RemnaPlus monorepo by
subtree split. Do not publish directly from a dirty or hand-edited checkout.

## Version Alignment

Before publishing, these values must agree:

| File | Field |
|------|-------|
| `internal/version/version.go` | `var Version` |
| `internal/version/contract.version` | RemnaPlus node contract |
| `scripts/install-node-alpine.sh` | `VERSION=` |
| `scripts/upgrade.sh` | `VERSION=` |
| `scripts/uninstall.sh` | `VERSION=` |
| RemnaPlus backend metadata | recommended Alpine tag |

Current release: `v1.0.1`. The immutable `v1.0.0` tag remains the previous known-good rollback target.

## Local Gates

```bash
go test ./...
bash -n scripts/*.sh deploy/remnawave-node-run.sh deploy/remnawave-node.openrc
bash scripts/install-node-alpine.test.sh
bash scripts/release-assets.test.sh
```

The monorepo publisher also runs the packaged Alpine gate before it pushes a
public tag.

## Release Assets

Pushing an immutable tag builds and uploads:

- `remnanode-lite_linux_amd64.tar.gz`
- `remnanode-lite_linux_arm64.tar.gz`
- `remnaplus-alpine-node-source-${tag}.tar.gz`
- `SHA256SUMS`

Operators install only from pinned tags. The installer downloads the selected
archive and `SHA256SUMS` from the same release and verifies the archive before
extracting it.

## Alpine Verification

On an Alpine/OpenRC VPS:

```bash
curl -fsSL https://raw.githubusercontent.com/12Jack21/remnaplus-alpine-node/v1.0.1/scripts/install-node-alpine.sh \
  -o /tmp/remnaplus-alpine-node.sh
RNL_TAG=v1.0.1 bash /tmp/remnaplus-alpine-node.sh --install --port 2222
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

To downgrade to a previous pinned release:

```bash
RNL_TAG=v1.0.0 bash scripts/upgrade.sh --yes
```
