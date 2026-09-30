# Release channels: stable, beta, dev

- **Status:** accepted
- **Date:** 2026-07-23

## Context

Users should be able to opt into pre-release builds. The bucket layout must support channels without complicating promotion, rollback, or the updater. Hive-style `channel=<name>/` key partitioning was considered and rejected: it serves query-engine scan pruning, and it encodes a mutable fact (which channel blesses a build) into immutable artifact paths — promoting a build would mean copying bytes to a second URL.

## Decision

Channels are mutable pointer files over an immutable artifact store (the Sparkle / electron-updater / Tauri convention):

```
desktop/releases/<full-semver>/          # immutable artifacts + SHA256SUMS
desktop/channels/{stable,beta,dev}/latest.json
```

Publishing rules:

1. **The version string routes the release.** The first semver prerelease identifier names the channel: `desktop-v1.5.0-dev.3` → dev, `desktop-v1.4.0-beta.2` → beta, bare `desktop-v1.4.0` → stable. The identifier is a closed set (`dev`, `beta`); the release workflow fails on anything else so a typoed tag cannot mint a channel.
2. **Publishes cascade to less-stable channels.** stable → stable+beta+dev; beta → beta+dev; dev → dev only. Semver keeps this safe: `dev` sorts above `beta` lexically, so a cascaded beta manifest never downgrades a dev user within the same base version, and a bare stable version outranks all its prereleases, converging everyone.
3. **The updater is channel-aware via one setting.** It fetches `channels/<channel>/latest.json` (default `stable`) and asserts `manifest.channel` matches its configured channel as defense-in-depth. A Settings toggle exposes beta/dev opt-in later.
4. **Rollback = rewrite `latest.json`** to a prior version directory; artifacts are never touched.

Dev-channel specifics: dev builds are still signed and notarized (unsigned builds fight Gatekeeper on every machine). Dev artifacts are retained by a prune job (delete `-dev.` versions older than N days) — R2 lifecycle rules are prefix-only and cannot match mid-key. Whether dev publishes stay manual or auto-fire on desktop-touching main merges is a later CI choice.

## Consequences

- Promotion, rollback, and channel membership are metadata operations; each build's bytes exist at exactly one URL.
- Channel behavior is fully determined by the git tag — no manual channel input to get wrong.
- The manifest shape is identical across channels, so the future admin server can serve per-license variants of the same scheme.
