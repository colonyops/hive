# Release notes ship inside the binary

- **Status:** accepted
- **Date:** 2026-08-06

## Context

After the app updates itself and relaunches, the version silently ticks up. The
user should be told what changed.

Three constraints decided the shape:

1. **The repository is private.** A user cannot read its GitHub releases, so
   GitHub is not a surface the app can link to or an API it can fetch from. The
   existing About ▸ Release link already 404s for anyone but the maintainer.
2. **Notes generated at publish time cannot describe the build that ships
   them.** The pipeline builds, signs, notarizes, uploads, then creates the
   GitHub release. Anything derived from the tag exists only after the binary
   is sealed.
3. **Prereleases are cut close to daily.** Dev and beta versions are numbered
   `-dev.N` / `-beta.N` and their number is chosen at release time from the tags
   *and* the live manifests, whose history predates this repository. A scheme
   that needs one authored file per published version therefore needs that
   file named for a version nobody can predict, landed through its own pull
   request, before every one of those releases.

The install path also rules out an install-time trigger: `InstallUpdate` ends by
relaunching, so the process that would raise the surface is gone.

## Decision

Release notes are markdown committed under
`internal/app/releasenotes/changelog/` and embedded with `go:embed`.

**Only a stable release gets an entry of its own**, at `<version>.md`. Everything
else accumulates in ~~`next.md`, the draft~~ the draft, which every build embeds
and which reads as "what this build has that no stable release does". A pull
request lands its changelog line by appending to the draft, where the reasoning
still exists; nothing is reconstructed from commit subjects at release time.

The draft is no longer one file: it is `changelog/unreleased/`, one file per
change, rendered at load (ADR release-notes-accumulate-as-fragments). Everything
below holds; only where the draft's bytes live has changed.

That makes a prerelease free to cut: `cmd/release` gates only stable on having
an entry, and a dev or beta release publishes the draft as it stands. Promotion
— `release changelog promote` moving the draft to `<version>.md` and stamping
the version and date — is the one moment notes and a version have to be tracked
together, and it is the moment the version is fully determined, since a stable
version is the bare base with no prerelease counter to guess.

Because every committed entry is a stable release, and the publish cascade sends
a stable release to every channel (ADR release-channels), there is nothing a user
on one channel must be kept from seeing. The read path carries no channel: an
entry naming a prerelease is rejected at parse time, which is what keeps that
true.

The surface is triggered at **launch**, not at install: the running version is
compared against a last-acknowledged version recorded in
`<StateDir>/releasenotes.json`. Newer means show the stable releases crossed to
get here, plus the draft. This also covers updates the app did not perform — a
package manager, a manual reinstall.

The marker lives in the state directory rather than `settings.yaml` because
`settings.yaml` is dotfiles-managed and shared between machines: having read the
notes is a fact about one installation, not a preference to carry.

**The modal is reserved for a launch that crossed a stable release.** A
prerelease bump carries only the draft — the same in-progress list the previous
build showed — and a modal repeating it on near-daily builds is hostile, so it
gets a toast whose action opens the full notes. An upgrade whose range carries
nothing degrades to the toast too, for a different reason: a modal with an empty
body says less than the line "Updated to X". The full history is always readable
in Settings ▸ About, draft first.

A *pending* update is the one case notes cannot come from the binary, since that
release is not installed. The channel manifest therefore carries `summary` and
`notes` for its version — the promoted entry for a stable release, the draft for
a prerelease — which is what populates `UpdateInfo.Notes`.

## Consequences

- Notes are readable offline, at first launch, with no network call — and a
  private repository stays private without costing users their changelog.
- Cutting a dev or beta release needs no changelog work at all. A stable release
  needs the draft promoted and committed first, and **will** fail without it.
- The changelog is a product history rather than a build log: one file per
  stable release, not one per prerelease.
- The draft repeats. A user tracking dev or beta is shown the same accumulating
  list on every bump, plus whatever landed since. That is accepted as the cost
  of not authoring per-build entries, and is why those bumps are a toast.
- ~~A stable release ships the draft's bytes unchanged, so what prerelease users
  were reading is what the release says.~~ Work that was reverted before the
  release has to be pruned from the draft at promotion, and since the draft
  became a set of fragments, promotion also consolidates and writes the summary
  (ADR release-notes-accumulate-as-fragments) — so a stable entry is edited from
  what prerelease users read, not copied from it.
- The GitHub release body is the same committed text, so the changelog and the
  release page cannot disagree.
- A build can only ever describe releases up to its own version. That is exactly
  what the What's New surface needs and is why the history list is not a
  substitute for the update check.
- A fresh install records the running version silently and shows nothing: there
  is no version it upgraded *from*.
