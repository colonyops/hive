# Release notes ship inside the binary

- **Status:** accepted
- **Date:** 2026-08-06

## Context

After the desktop app updates itself and relaunches, the user should be told
what changed, offline and with no network call. Notes generated at publish time
cannot describe the build that ships them: anything derived from the tag exists
only after the binary is sealed. An install-time trigger is out too, because
`InstallUpdate` ends by relaunching.

## Decision

Release notes are markdown committed under
`cmd/desktop/releasenotes/changelog/` and embedded with `go:embed`. Each
released version has a `<version>.md`; unreleased work accumulates as the draft
in `changelog/unreleased/` (ADR release-notes-accumulate-as-fragments), which
every build embeds as "what this build has that no release does". A pull
request adds its own note, while the reasoning still exists. Nothing is
reconstructed from commit subjects at release time.

`release changelog promote` turns the draft into `<version>.md` and is where
notes and a version are tied together. A release fails without an entry. An
entry that names a prerelease is rejected at parse time.

The surface is triggered at **launch**: the running version is compared with
the last acknowledged version in `<StateDir>/releasenotes.json`. Newer means
show the releases crossed to get here, plus the draft. This also covers updates
the app did not perform (a package manager, a manual reinstall). The marker is
in the state directory, not `settings.yaml`, because `settings.yaml` is
dotfiles-managed and shared between machines, and having read the notes is a
fact about one installation.

**The modal is reserved for a launch that crossed a release.** A range that
holds only the draft, or nothing, gets a toast whose action opens the full
notes: repeating the same in-progress list in a modal is a nuisance, and an
empty modal says less than "Updated to X". The full history is in Settings >
About, draft first.

A pending update is not installed, so its notes cannot come from the binary.
The channel manifest carries `summary` and `notes` for its version, which
populate `UpdateInfo.Notes`.

## Consequences

- A build can only describe releases up to its own version. That is what the
  What's New surface needs, and why the history list is not a substitute for
  the update check.
- A fresh install records the running version silently and shows nothing.
- The modal acknowledges on dismissal and the toast on sight, so a crash while
  the modal is open does not cost the user the notes.
- The GitHub release body is the same committed text
  (ADR every-program-keeps-its-own-release-notes-and-promotes-them-under-one-version),
  so the changelog and the release page cannot disagree.
