# macOS ships an installer disk image beside the update zip

- **Status:** accepted
- **Date:** 2026-08-03

## Context

macOS published one artifact: a zip of the bare `Hive.app` ([r2-manifest-distribution](2026-07-23-r2-manifest-distribution.md)). That is the right shape for the updater and the wrong one for a person. Someone who downloads it gets an app sitting in `~/Downloads` with no install affordance, and running it from there is actively worse than it looks — Gatekeeper *translocates* a quarantined app, so the running bundle is a read-only mount at a random path. The updater then cannot replace the app it is running, and the first self-update of a fresh install fails.

The zip cannot simply be replaced. The Wails updater declares its artifact filetype as `zip` and unpacks it in place; it has no disk-image path. Every already-shipped build reads `platforms.<key>.url` and expects a zip there, so that field is frozen by builds we no longer control.

## Decision

**Publish `Hive-<ver>-darwin-universal.dmg` alongside the zip, and split the manifest entry by role: `url` is what the updater downloads, `installer_url` is what a human downloads.**

1. **`hdiutil` directly, no DMG toolchain.** The image is built from a staging folder holding the app, an `/Applications` symlink, and `.VolumeIcon.icns`. A helper like `create-dmg` would add a dependency to the release machine for a wrapper over the same commands.
2. **No Finder-scripted window layout.** Icon positions and a background image are only settable by driving Finder over AppleScript against the mounted volume, which is TCC-gated, needs a logged-in session, and would fail a release for a permissions prompt. The volume gets its icon; Finder auto-arranges the two items, and the app beside the `/Applications` shortcut is the affordance that matters.
3. **The image is signed, notarized, and stapled in its own right.** Gatekeeper evaluates the container the user opens, so a stapled app inside an unnotarized image still warns. That means two Apple round trips per release, not one — the app is notarized and stapled first, then the image is built from that stapled app and notarized itself. Notarizing only the image would leave the installed copy in `/Applications` without a ticket, needing a network check on first launch.
4. **`installer_url` + `installer_sha256` + `installer_size`, optional, read as a set.** Optional because manifests published before this decision must keep parsing, and because Linux has no separate installer. Read as a set because a consumer that took the URL and skipped a missing checksum would install unverified bytes; `cmd/release` rejects a partially populated group.
5. **The installer follows the whole manifest.** The channel cascade writes one manifest per affected channel naming every artifact, so beta and dev get the installer for free — there is no per-artifact channel routing to get wrong.

## Consequences

- **A release now waits on Apple twice.** The two submissions are sequential by construction: the image is built from the already-stapled app. This is the cost of item 3 and there is no way around it short of giving up an offline-validating installed app.
- **`SetFile` joins the required toolchain.** The custom-icon flag lives in the volume's Finder info, so the image is built writable, mounted, flagged, unmounted, and only then converted to compressed read-only. `SetFile` ships with Xcode, which a release machine already needs; `preflight` fails early if it is missing rather than mid-release.
- **The layout is asserted, not assumed.** Nothing in `hdiutil` fails if the symlink lands as a copied directory or the icon is dropped — the image builds fine and the user gets a window they cannot install from. So the publisher mounts the finished image and checks the app, the symlink's target, the volume icon, the code signature, and the stapled ticket, and a unit test builds and mounts a real image from a stub app to cover the same path without signing. The test is macOS-only, which matches where releases are cut ([linux-tarball-distribution](2026-07-27-linux-tarball-distribution.md)); the Linux CI runner skips it.
- **The install script stays on the zip.** It installs headlessly and `unzip` is one command against no mount; resolving the installer would mean attaching and detaching a volume for no gain ([install-script](2026-07-27-install-script.md)).
- **The landing page's download CTA is not wired up yet.** The site is in private beta and its primary CTA is the invite form, so nothing there points at an artifact today. When it does, it reads `installer_url` — the `/api/latest` worker proxy passes the manifest through untouched, so no worker change is needed.
- Two artifacts per macOS release roughly doubles the bucket cost of a version. Retention already prunes dev builds, and that is the channel that produces most of them.
