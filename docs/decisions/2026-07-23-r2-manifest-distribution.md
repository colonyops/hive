# Distribution and auto-update via R2 + channel manifests

- **Status:** accepted
- **Date:** 2026-07-23

## Context

This repo is private, so GitHub Releases cannot serve public downloads, and the in-app updater would need a PAT on every user machine. Distribution needs public, unauthenticated HTTPS with URLs stable enough to bake into shipped binaries. App-update downloads are pure egress.

## Decision

Releases are distributed from a Cloudflare R2 bucket behind a domain we own:

- **Versioned, immutable artifacts** under `desktop/releases/<semver>/` plus a small **mutable manifest** per channel (`desktop/channels/<channel>/latest.json`: version, per-platform URL, sha256, size). The app fetches the manifest — it never lists the bucket and holds no credentials.
- **The fronting domain is the hard requirement:** the manifest URL baked into binaries is `dl.hivedesktop.com`, never a raw bucket URL, so storage stays swappable forever.
- **R2 over AWS S3:** S3-compatible API (identical tooling) with zero egress fees, and it pairs with the Cloudflare-hosted landing page.
- **Updater:** the Wails `pkg/updater` `Provider` is implemented as a simple static-manifest poller (GET manifest → semver compare → download → verify sha256), replacing the custom GitHub provider and its tag-namespace workarounds. TLS + sha256-in-manifest is the integrity baseline; the notarized Developer ID signature is the trust anchor. Hardening follow-up: sign the manifest (minisign/EdDSA) so a compromised bucket cannot push updates.
- **Cache rules:** artifacts `public, max-age=31536000, immutable`; manifests `no-cache`. Set per-object at upload.
- Git tags (`desktop-v*`) remain the source-side version anchor and changelog record; nothing user-facing reads GitHub.

## Consequences

- Landing-page downloads, auto-update, and (later) license-aware serving all flow through one URL scheme; the future admin server can take over manifest serving without moving the artifact store.
- Rollback and channel management are manifest rewrites, never artifact changes.
- Release CI needs R2 write credentials (dashboard-created API token).
