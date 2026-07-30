# RemnaPlus Alpine Node Contract

This repository implements the `ALPINE_NATIVE` node runtime for RemnaPlus. The dashboard persists the runtime in dashboard node data; operators and backend services must not infer it from a reported node version. `STANDARD_DOCKER` nodes continue to use the TypeScript Remnanode container.

The Alpine runtime supports the approved RemnaPlus node API, including Xray lifecycle and statistics, user and inbound management, plugins, direct TCP observations, bounded audit-log access, native vnStat telemetry, and Reality SNI health probes.

## Forwarding Exclusion

An Alpine node cannot be a forwarding source or a forwarding target. It does not install the middle-forward stack, mount an HAProxy socket, or publish HAProxy forwarding telemetry. The route `/node/stats/get-forwarded-tcp-connections` is intentionally absent.

The dashboard enforces this boundary when forwarding links are created or updated and before a disabled node changes to `ALPINE_NATIVE`. Legacy links involving an Alpine node are excluded defensively from telemetry collection.

## Runtime Selection

New nodes default to `STANDARD_DOCKER`. An operator may select `ALPINE_NATIVE` during creation or change a disabled, unlinked node later. Runtime-specific installation instructions are generated from the pinned Alpine release repository and tag returned by dashboard metadata.
