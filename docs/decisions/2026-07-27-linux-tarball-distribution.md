# Linux ships as a tarball, not a package

- **Status:** accepted; point 5 added on 2026-09-21 after Linux install testing
- **Date:** 2026-07-27

## Context

Linux needed to become a published target alongside macOS ([r2-manifest-distribution](2026-07-23-r2-manifest-distribution.md), [release-channels](2026-07-23-release-channels.md)). The wails scaffold already carried build assets for four formats — AppImage, `.deb`, `.rpm`, and an Arch package via nfpm — so the question was which of them to actually publish, and whether the in-app updater could keep working across them.

Two properties of the wails updater constrain the answer, and neither is visible on macOS:

- Its extractor **rejects archives with more than one top-level entry**, and its helper renames whatever it extracts directly over the running executable (`bundleTarget(exe) == exe` off darwin). So the updatable artifact must be an archive whose single top-level entry *is* the binary. A wrapper directory would replace the executable with a directory.
- `os.Executable()` inside an AppImage resolves into the FUSE mount (`/tmp/.mount_XXXX/AppRun`), not the `.AppImage` file. **An AppImage cannot self-update** through this updater at all.

Packaged installs add a second problem: `.deb`/`.rpm` land the binary in a root-owned prefix, so a self-update by a normal user cannot write the backup or the replacement. Those installs must defer to the package manager, which means publishing them commits us to a repository (apt/dnf), signing keys, and the operational surface that follows — while the product is in early alpha with a handful of users.

The first external Linux install also exposed two tarball-specific failures: a missing WebKitGTK runtime surfaced only when the binary started, and the installed app had no application-menu entry. Neither requires a package format when the terminal installer can detect the linked libraries and write user-level XDG integration.

## Decision

**Publish one Linux artifact per architecture: a `.tar.gz` containing a single `hive-desktop` binary, for `linux-amd64` and `linux-arm64`.**

1. **No package formats.** No `.deb`, `.rpm`, AppImage, or Arch package, and no apt/dnf repository. The nfpm config and the AppImage/deb/rpm/aur tasks stay in `desktop/build/linux/` unused — reviving one is a scoped task, not a rewrite. Revisit when there is demand that a tarball genuinely fails to serve.
2. **No GPG signing.** Signing exists to establish trust through a *repository*; with direct downloads, the manifest sha256 (verified by the updater) and the release's `SHA256SUMS` (for humans) already cover integrity, over TLS from a domain we control. Signing without a repo would add key management for no threat actually mitigated.
3. **`linux-amd64` and `linux-arm64`.** Both are published. Development happens on an Apple Silicon Mac, where the arm64 container build is native and fast, and testing the real in-app update path in an arm64 Linux VM requires a matching manifest entry — without one the updater reports no artifact for the platform. amd64 is the emulated, slow one locally.
4. **Install under a user-owned prefix.** `~/.local/bin` is what the manual-install docs recommend, because it is what keeps in-app updates working. The terminal installer uses `~/.local/share/hive` and keeps that directory user-owned for the same reason.
5. **The terminal installer supplies user-level desktop integration.** It checks the downloaded binary for missing runtime libraries, writes a `.desktop` entry and icon under the XDG data directory, and refreshes the desktop database when the tool is available. It links to the dependency instructions but does not run a package manager or `sudo` on the user's behalf.
6. **A release publishes every platform at once, from one machine.** `release publish` builds macOS natively and both Linux architectures in a container, then writes one manifest per affected channel naming all three. There is no CI publishing workflow.

## Consequences

- The updater's single-top-level-entry rule becomes a **release-time invariant**: `cmd/release` builds the tarball itself and re-reads it, failing the publish unless it holds exactly one entry, that entry is the binary, and the executable bit survived. Breaking it would otherwise be invisible until users tried to update.
- Two properties of the helper needed runtime work in `internal/adapter/wailsui/updater_staging.go`, both Linux-only. Staging is redirected to the binary's own directory for the duration of a download, because the helper's rename has no cross-device fallback and `/tmp` is tmpfs on Fedora, Arch, openSUSE, RHEL 9+, and Debian 13+ — `EXDEV` would fail every update there. And a non-writable install is detected before downloading, rather than after.
- **Publishing is local-only, and there is no CI publish workflow.** GitHub-hosted macOS runners have no Docker, so a macOS job cannot build Linux at all; splitting the work across an Ubuntu job and a macOS one would mean two publishes of one version, which the manifest-advancement rule rejects by design (the second does not advance the version the first just set). Building everything on one machine keeps that rule intact and the release atomic: one manifest write naming every platform. Releases are cut with `mise run release` from a Mac with Docker running.
- Consequently a release now needs macOS **and** Docker on the same machine, and cutting one costs an emulated amd64 Linux compile. The builder image is cached per architecture, so that price is paid per image, not per release.
- A raw tarball still has no launcher entry, icon registration, or managed dependency install. The terminal installer supplies the first two and links to the Linux requirements when GTK4 or WebKitGTK 6.0 is missing.
- Two further Linux-only runtime gaps surfaced in the same audit and are fixed alongside: the macOS tray asset is a pure-black *template* icon that Linux renders verbatim (invisible on a dark panel), so Linux gets a white render; and close-to-tray strands the app on sessions with no StatusNotifier host, so that behaviour is now conditional on one being present.
