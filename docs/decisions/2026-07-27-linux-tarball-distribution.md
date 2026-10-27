# Linux ships as a tarball, not a package

- **Status:** accepted
- **Date:** 2026-07-27

## Context

Linux is a published target beside macOS
([r2-manifest-distribution](2026-07-23-r2-manifest-distribution.md),
[release-channels](2026-07-23-release-channels.md)). The Wails updater
constrains the artifact format:

- Its extractor rejects archives with more than one top-level entry, and off
  darwin its helper renames what it extracts directly over the running
  executable. The updatable artifact must be an archive whose single entry is
  the binary.
- Inside an AppImage, `os.Executable()` resolves into the FUSE mount, so an
  AppImage cannot self-update.
- `.deb` and `.rpm` installs land in a root-owned prefix a user's self-update
  cannot write, and publishing them commits us to an apt/dnf repository and
  signing keys.

## Decision

**Publish one `.tar.gz` per architecture (`linux-amd64`, `linux-arm64`),
containing a single `hive-desktop` binary.**

1. **No package formats** (`.deb`, `.rpm`, AppImage, Arch) and no package
   repository. The nfpm config under `cmd/desktop/build/linux/nfpm` stays,
   unused. Revisit when a tarball fails real demand.
2. **No GPG signing.** Without a repository it mitigates nothing the manifest
   sha256 (checked by the updater) and the release's `SHA256SUMS` do not
   already cover over TLS.
3. **Install under a user-owned prefix** so in-app updates keep working:
   `~/.local/bin` for a manual install, `~/.local/share/hive` for the terminal
   installer (ADR install-script).
4. **The terminal installer supplies user-level desktop integration.** It
   checks the binary for missing runtime libraries, writes a `.desktop` entry
   and icon under the XDG data directory, and links to the dependency
   instructions. It never runs a package manager or `sudo`.
5. **A release publishes every platform at once, from one machine.** The
   `release` tool builds macOS natively and both Linux architectures in Docker
   (`cmd/desktop/scripts/build/build-linux-docker.sh`), then writes one
   manifest per channel naming all of them. No CI workflow publishes the
   desktop.

## Consequences

- The single-entry rule is a release-time invariant: `cmd/tools/release`
  builds the tarball itself and re-reads it, failing unless it holds exactly
  one entry, that entry is the binary, and the executable bit survived.
- `cmd/desktop/internal/adapter/wailsui/updater_staging.go` stages a download
  in the binary's own directory on Linux, because the helper's rename has no
  cross-device fallback and `/tmp` is tmpfs on many distros (`EXDEV`). It also
  detects a non-writable install before downloading.
- Desktop publishing stays local: GitHub-hosted macOS runners have no Docker,
  and splitting macOS and Linux across two jobs means two publishes of one
  version, which the manifest-advancement rule rejects. A release needs macOS
  and Docker on the same machine.
