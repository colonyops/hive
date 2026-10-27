# A settings.yaml key the build does not declare is ignored, not rejected

- **Status:** accepted
- **Date:** 2026-10-02

## Context

A newer build that adds a field writes it on its next save. A config
directory synced between machines then lands that file on every machine still
on an older build. If the older build refuses to start, the in-app updater is
unreachable and each machine needs a binary installed by hand. Migrations do
not help: they run forward only, and an additive field does not bump the
version.

## Decision

**A key the build does not declare loads and is ignored.** The settings
loader decodes leniently; after that succeeds, a strict decode into a throwaway value
collects the unknown keys (`Settings.UnknownKeys`), and startup logs them once
as a warning. The updater must stay reachable from a file this build cannot
fully parse.

**A type mismatch is still an error**, and a file whose `version` is above the
build's `Current` is still rejected. A version bump is a declared breaking
change.

**An older build's save drops the keys it does not know.** The cost is one
setting reverting to its default on the newer machine while the older one
stays behind, which the reachable updater ends.

## Consequences

- A misspelled key does not fail startup. It is logged and does nothing; the
  `hive-settings` skill says so.
- A new field needs no version bump. A rename or removal still does.
- Flow, actions, and agent-workspace files keep their strict decode; their
  last-good reload already makes a bad file non-fatal.
