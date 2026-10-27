# One-line install script served by the site

- **Status:** accepted
- **Date:** 2026-07-27

## Context

Users need to go from nothing to a running app without hunting for the right
artifact. The in-app updater already resolves builds from channel manifests
([r2-manifest-distribution](2026-07-23-r2-manifest-distribution.md),
[release-channels](2026-07-23-release-channels.md)). First install should
resolve the same way so the two never disagree on version or artifact.

## Decision

1. **A `curl | bash` one-liner at `https://hivedesktop.com/install.sh`.** The
   script is `docs/docs/install.sh`, copied unchanged to the site root by the
   docs build. No separate hosting.

2. **Resolve from the channel manifest the updater reads**
   (`dl.hivedesktop.com/desktop/channels/<channel>/latest.json`, `stable` by
   default, `--channel` or `HIVE_CHANNEL` to override), and verify the
   artifact's sha256 against the manifest before installing. A mismatch
   aborts.

3. **macOS installs to `/Applications`**, falling back to `~/Applications`
   when it is not writable, from a Developer ID-signed, notarized build. A
   `curl` download carries no quarantine bit, so no `xattr` step is needed.

4. **Linux installs under a user-owned prefix** (`~/.local/share/hive`, or
   `HIVE_HOME`) so in-app updates keep working (ADR
   linux-tarball-distribution).

5. **Re-running upgrades in place**, so the script is also a manual update
   path.

## Consequences

- The install URL is public and appears in the README, the site, and the
  docs. Changing it breaks published copy.
- The script itself is unsigned and unversioned. Its integrity rests on HTTPS
  from a domain we control, the manifest checksum of the artifact, and being
  short enough to read before running.
