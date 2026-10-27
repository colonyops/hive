# Distribution and auto-update via R2 + channel manifests

- **Status:** accepted
- **Date:** 2026-07-23

## Context

Desktop downloads and auto-update need public, unauthenticated HTTPS with URLs stable enough to bake into
shipped binaries, and the in-app updater must hold no credentials. App-update downloads are pure egress.

## Decision

Desktop releases are served from a Cloudflare R2 bucket behind a domain we own:

- **Versioned, immutable artifacts** under `desktop/releases/<version>/` plus a small **mutable manifest**
  per channel (`desktop/channels/<channel>/latest.json`: version, per-platform URL, sha256, size). The app
  fetches the manifest; it never lists the bucket.
- **The fronting domain is the hard requirement.** Binaries bake in `https://dl.hivedesktop.com`, never a
  raw bucket URL, so storage stays swappable without an app update.
- **R2 over S3** for the S3-compatible API with zero egress fees.
- **Updater:** the Wails `pkg/updater` `Provider` is a static-manifest poller
  (`cmd/desktop/internal/adapter/wailsui/updater_provider.go`): GET manifest, semver compare, download,
  verify sha256. TLS plus the manifest sha256 is the integrity baseline; the notarized Developer ID
  signature is the trust anchor. The manifest itself is not signed.
- **Cache rules** are set per object at upload: artifacts `public, max-age=31536000, immutable`,
  manifests `no-cache`.

## Consequences

- Rollback and channel management are manifest rewrites, never artifact changes. `cmd/tools/release`
  refuses to overwrite an existing release prefix without `--force`.
- Publishing needs R2 write credentials (`R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`) on the machine that
  runs the release.
- `docs/distribution.md` holds the bucket, domain, and manifest schema.
