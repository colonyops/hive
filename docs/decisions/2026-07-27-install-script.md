# One-line install script served by the landing-page worker

- **Status:** accepted; point 3 reversed on 2026-09-05 — the repository is
  public, so the path token protects nothing and the script now sits at
  `/install.sh`
- **Date:** 2026-07-27

## Context

Alpha testers need to go from nothing to a running app without hunting through
R2 for the right artifact. The in-app updater already resolves builds from
channel manifests ([r2-manifest-distribution](2026-07-23-r2-manifest-distribution.md),
[release-channels](2026-07-23-release-channels.md)); first install should resolve the same way so
the two never disagree on version or artifact. We want no new infrastructure,
and the link kept out of casual discovery while the beta — and the private repo
that holds the link — stay private. macOS is the only published platform; the
Linux installer half is left to the Linux publishing work (#36).

## Decision

1. **A `curl | bash` one-liner, served as a static asset by the existing
   landing-page worker ([web-workers-static-assets](2026-07-23-web-workers-static-assets.md))** and shipped
   by `deploy-web.yml`. No new worker, bucket, or domain.

2. **Resolve from the same channel manifest the updater reads**, verify the
   downloaded artifact's sha256 against the manifest before touching disk, and
   abort on mismatch. Install and auto-update agree by construction, and a
   corrupted or swapped artifact never installs.

3. ~~**Behind an obscure path token, nested under the existing invite-page
   token** (`/install/<token>/install.sh`).~~ **Reversed 2026-09-05.** The token
   was obscurity, not authentication, and it held only while the repo was
   private. The repo is public, so the token is in git history and protects
   nothing. The script is `/install.sh` and the page is `/install`; both are
   crawlable and the page is in the sitemap.

4. **macOS installs to `/Applications`** (falling back to `~/Applications`
   without sudo) from a Developer ID-signed, notarized, stapled build, so
   Gatekeeper passes cleanly with no `xattr` workaround — and a `curl` download
   carries no quarantine bit to strip in the first place. Re-running upgrades in
   place, so the script doubles as a manual update path when the in-app updater
   is unavailable.

## Consequences

- The install URL is stable and public. It appears in the README, on the site,
  and in the docs, so changing it breaks published copy in all three.
- The script itself is unsigned and unversioned; its integrity rests on HTTPS
  from a domain we control, the in-manifest checksum of the artifact it
  downloads, and being short enough to read (`| less`) before running.
- Linux is out of scope here: platform detection and an install branch are
  stubbed and documented in the script, but making them real — matching the
  tarball format, keys, and user-owned prefix — belongs to the Linux publishing
  work (#36).
