# Distribution and auto-update via GitHub release assets + R2 channel manifests

- **Status:** accepted
- **Date:** 2026-07-23

## Context

Desktop downloads and auto-update need public, unauthenticated HTTPS with URLs stable enough to bake into
shipped binaries, and the in-app updater must hold no credentials. App-update downloads are pure egress,
and keeping every release's artifacts in our own bucket makes storage grow without bound.

## Decision

Desktop artifacts are assets of the version's GitHub release. The channel manifests stay in a Cloudflare
R2 bucket behind a domain we own:

- **Immutable artifacts on GitHub.** Each release attaches the desktop artifacts and `SHA256SUMS` to the
  one GitHub release for `v<version>`, beside the CLI archives. A manifest names their
  `github.com/colonyops/hive/releases/download/v<version>/` URLs.
- **A small mutable manifest in R2** per channel (`desktop/channels/<channel>/latest.json`: version,
  per-platform URL, sha256, size). The app fetches the manifest; it never lists the bucket. R2 holds only
  the manifests, so the bucket does not grow with each release.
- **The fronting domain is the hard requirement.** Binaries bake in `https://dl.hivedesktop.com` for the
  manifests, never a raw bucket URL, so manifest storage stays swappable without an app update. Artifact
  URLs come from the manifest, so they can move without one.
- **Updater:** the Wails `pkg/updater` `Provider` is a static-manifest poller
  (`cmd/desktop/internal/adapter/wailsui/updater_provider.go`): GET manifest, semver compare, download
  (following GitHub's redirect to its asset host), verify sha256. TLS plus the manifest sha256 is the
  integrity baseline; the notarized Developer ID signature is the trust anchor. The manifest itself is not
  signed.
- **Manifests are `no-cache`**, set at upload.

## Consequences

- Rollback and channel management are manifest rewrites, never artifact changes. `cmd/tools/release`
  refuses to replace an existing release asset without `--force`.
- The GitHub release has to exist, published and not a draft, before a manifest names its assets, so the
  desktop publish creates it and the publish workflow adds the CLI archives to it.
- Immutable releases must stay off for the repository: the workflow attaches the CLI archives after the
  release is published.
- Releases published before this change keep their artifacts under `desktop/releases/<version>/` in R2.
  A rollback to one of them needs those objects.
- Publishing needs R2 write credentials (`R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`) and an authenticated
  `gh` on the machine that runs the release.
- `docs/distribution.md` holds the bucket, domain, and manifest schema.
