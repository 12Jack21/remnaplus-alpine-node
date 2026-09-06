# RemnaPlus Native Node

RemnaPlus's native Go node for Alpine Linux/OpenRC and Debian 12/systemd. It
installs as a single static binary and runs directly on the VPS without Docker.

This repository is the public release mirror for the canonical source in the
private RemnaPlus monorepo. Published tags contain the complete corresponding
AGPL source and immutable `linux/amd64` and `linux/arm64` release artifacts.

## Support

- Alpine Linux with OpenRC
- Debian 12 with systemd
- `amd64` and `arm64`
- Remnawave node contract `2.8.0`
- Direct node telemetry, plugins, audit APIs, and Reality SNI health

Native nodes cannot be forwarding sources or targets and do not expose HAProxy
forwarding telemetry. Debian VPS nodes may still use the standard Docker
Remnanode when the native runtime is not selected.

## Install

Create an `ALPINE_NATIVE` node in the RemnaPlus dashboard and use the pinned
Chinese or English install command shown there. Both entry points run the same
installer engine; only operator-facing messages differ.

The current public release is `v1.0.3`. Source `v1.0.4` is a canary candidate
and must not be installed from GitHub until its immutable publication gate
passes. Dashboard launchers request the Secret Key separately so credentials
are not embedded in a long pasted command.

For the current public Alpine release:

```bash
curl -fsSL 'https://raw.githubusercontent.com/12Jack21/remnaplus-alpine-node/v1.0.3/scripts/install-node-alpine.sh' \
  -o '/tmp/remnaplus-alpine-node.sh' &&
read -rsp 'Node Secret Key: ' NODE_SECRET; echo
umask 077; printf '%s' "$NODE_SECRET" > /tmp/remnanode-secret.key; unset NODE_SECRET
env RNL_TAG='v1.0.3' bash '/tmp/remnaplus-alpine-node.sh' \
  --install --yes --port 2222 --secret-file /tmp/remnanode-secret.key
rm -f /tmp/remnanode-secret.key
```

Use `scripts/install-node-alpine-en.sh` for English output. The dashboard's
generated flow keeps the existing Node Secret Key separate from the launcher;
keep it private and enter it only in the target native VPS root shell.

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
