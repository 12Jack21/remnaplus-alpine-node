# RemnaPlus Native Go Node Contract

This repository implements the native Go node runtime for RemnaPlus on Alpine/OpenRC and Debian/systemd. The dashboard persists the runtime in dashboard node data; operators and backend services must not infer it from a reported node version. `STANDARD_DOCKER` nodes continue to use the TypeScript Remnanode container.

The native runtime supports the approved RemnaPlus node API, including Xray lifecycle and statistics, user and inbound management, plugins, Vision IP rules, direct TCP observations, bounded audit-log access, native vnStat telemetry, WARP health, and Reality SNI health probes.

## Forwarding telemetry

Native nodes expose `/node/stats/get-forwarded-tcp-connections` when the host forwarding paths are configured. The process reads `/etc/middle-forward/mappings.tsv` and `/run/middle-forward/haproxy.sock` directly, with the same TCP PROXY-protocol mapping and HAProxy stick-table diagnostics as the Docker node. Native installation does not silently install HAProxy; operators must deploy the forwarding producer and set `MIDDLE_FORWARD_TELEMETRY=1` with the corresponding host paths.

The dashboard may use a native node as a forwarding source or target after the host forwarding prerequisites are installed. Missing or stale forwarding paths produce scoped telemetry diagnostics rather than attributing traffic silently.

## Triple-network backtrace

Native nodes expose `POST /node/backtrace/run` for the dashboard Node Monitor backtrace panel. The node validates each checkpoint (public IPv4 only, DNS-rebinding resistant), runs the pinned `nexttrace-tiny` binary once with JSON output for hop classification and once in plain-text mode for the raw panel output, and returns at most three probes. The installers place `nexttrace-tiny` under `/usr/local/bin`; `NEXTTRACE_PATH` may override the location. Missing binaries or blocked raw-socket capabilities are reported per probe rather than failing the request.

## Runtime Selection

New nodes default to `STANDARD_DOCKER`. An operator may select `ALPINE_NATIVE` or `DEBIAN_NATIVE` during creation, or change a disabled node later. Runtime-specific installation instructions are generated from the pinned native release repository and tag returned by dashboard metadata.
