# macOS ships an installer disk image beside the update zip

- **Status:** accepted
- **Date:** 2026-08-03

## Context

The macOS update artifact is a zip of the bare `Hive.app` (ADR
r2-manifest-distribution). For a person, that leaves an app in `~/Downloads`
with no install affordance. Running it from there is worse than it looks:
Gatekeeper translocates a quarantined app to a read-only mount at a random
path, so the first self-update of a fresh install fails.

The zip cannot be replaced. The Wails updater unpacks a `zip` in place and has
no disk-image path, and every shipped build reads `platforms.<key>.url` expecting
a zip.

## Decision

**Publish `Hive-<ver>-darwin-universal.dmg` beside the zip, and split the
manifest entry by role: `url` is what the updater downloads, `installer_url` is
what a human downloads** (`cmd/tools/release`).

1. **`hdiutil` directly, no DMG toolchain.** The image is built from a staging
   folder holding the app, an `/Applications` symlink and `.VolumeIcon.icns`.
2. **No Finder-scripted window layout.** Icon positions and backgrounds are
   only settable by driving Finder over AppleScript, which is TCC-gated and
   would fail a release on a permissions prompt.
3. **The image is signed, notarized and stapled in its own right.** Gatekeeper
   evaluates the container the user opens. The app is notarized and stapled
   first, then the image is built from it and notarized, so the installed copy
   in `/Applications` carries its own ticket.
4. **`installer_url`, `installer_sha256` and `installer_size` are optional and
   read as a set.** Optional, because older manifests must keep parsing and
   Linux has no separate installer. A set, because a consumer that took the URL
   without a checksum would install unverified bytes; the release tool rejects
   a partial group.
5. **The installer follows the whole manifest**, so every channel the cascade
   writes gets it.

## Consequences

- A release waits on Apple twice, sequentially, because the image is built from
  the stapled app.
- `SetFile` is a required release tool. The custom-icon flag lives in the
  volume's Finder info, so the image is built writable, mounted, flagged,
  unmounted, then converted to compressed read-only. `preflight` checks for it.
- The layout is asserted, not assumed. `hdiutil` succeeds even if the symlink
  lands as a directory or the icon is dropped, so the publisher mounts the
  finished image and checks the app, the symlink target, the volume icon, the
  signature and the stapled ticket. A macOS-only unit test builds and mounts a
  real image from a stub app.
- `install.sh` stays on the zip, which installs headlessly with no mount (ADR
  install-script). The site's download buttons (`docs/docs/javascripts/download.js`)
  read the `installer_*` fields.
