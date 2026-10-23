# Release channels: stable, beta, dev

- **Status:** accepted
- **Date:** 2026-07-23

## Context

Users should be able to opt into pre-release builds. The bucket layout must support channels without
complicating promotion, rollback, or the updater, and a build's bytes should exist at one URL only.

## Decision

Channels are mutable pointer files over an immutable artifact store:

```
desktop/releases/<version>/          # immutable artifacts + SHA256SUMS
desktop/channels/{stable,beta,dev}/latest.json
```

- **The updater is channel-aware via one setting** (`updates.channel`, default `stable`). It fetches
  `channels/<channel>/latest.json` and rejects a manifest whose `channel` field does not match its
  configured channel.
- **Rollback is a rewrite of `latest.json`** to a prior version directory; artifacts are never touched.

Which manifests a release writes is decided by
[every-program-ships-under-one-shared-version](2026-10-01-every-program-ships-under-one-shared-version.md):
every release writes all three, so the channels currently carry the same build.

## Consequences

- Promotion, rollback, and channel membership are metadata operations.
- The manifest shape is identical across channels.
