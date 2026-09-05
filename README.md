# RemnaPlus Alpine Node

RemnaPlus's native Go node for Alpine Linux and OpenRC. It installs as a
single static binary and runs directly on the VPS without Docker.

This repository is the public release mirror for the canonical source in the
private RemnaPlus monorepo. Published tags contain the complete corresponding
AGPL source and immutable `linux/amd64` and `linux/arm64` release artifacts.

## Support

- Alpine Linux with OpenRC
- `amd64` and `arm64`
- Remnawave node contract `2.8.0`
- Direct node telemetry, plugins, audit APIs, and Reality SNI health

Non-Alpine VPS nodes must use the standard Docker Remnanode supplied by
RemnaPlus. Alpine-native nodes cannot be forwarding sources or targets and do
not expose HAProxy forwarding telemetry.

## Install

Create an `ALPINE_NATIVE` node in the RemnaPlus dashboard and use the pinned
Chinese or English install command shown there. Both entry points run the same
installer engine; only operator-facing messages differ.

For the current `v1.0.3` release:

```bash
curl -fsSL 'https://raw.githubusercontent.com/12Jack21/remnaplus-alpine-node/v1.0.3/scripts/install-node-alpine.sh' \
  -o '/tmp/remnaplus-alpine-node.sh' &&
env RNL_TAG='v1.0.3' SECRET_KEY='<redacted>' bash '/tmp/remnaplus-alpine-node.sh' \
  --install --yes --port 2222
```

Use `scripts/install-node-alpine-en.sh` for English output. The dashboard's
generated command supplies the existing Node Secret Key non-interactively;
keep that command private and run it only in the target Alpine root shell.

The service configuration is stored in `/etc/remnanode/node.env`, state in
`/var/lib/remnanode`, and logs in `/var/log/remnanode`.

## Operations

```bash
sudo rc-service remnawave-node status
sudo tail -f /var/log/remnanode/node.log
sudo remnanode-lite doctor
```

The release workflow and installer are hardened further in the monorepo before
the first public tag is published. Do not install from an unpinned branch.

## Development

```bash
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath ./cmd/remnanode-lite
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath ./cmd/remnanode-lite
```

## License

AGPL-3.0-only. See [LICENSE](LICENSE).
