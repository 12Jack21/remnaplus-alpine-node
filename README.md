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

Debian and other systemd distributions must use the standard Docker
Remnanode supplied by RemnaPlus. Alpine-native nodes cannot be forwarding
sources or targets and do not expose HAProxy forwarding telemetry.

## Install

Create an `ALPINE_NATIVE` node in the RemnaPlus dashboard and use the pinned
install command shown there. The installer asks for the node `SECRET_KEY`
separately; the generated command never embeds credentials.

For the initial `v1.0.0` release:

```bash
curl -fsSL https://raw.githubusercontent.com/12Jack21/remnaplus-alpine-node/v1.0.0/scripts/install-node-alpine.sh \
  -o /tmp/remnaplus-alpine-node.sh
sudo RNL_TAG=v1.0.0 bash /tmp/remnaplus-alpine-node.sh --install --port 2222
```

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
