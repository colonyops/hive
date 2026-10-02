# A settings.yaml key the build does not declare is ignored, not rejected

- **Status:** accepted
- **Date:** 2026-10-02

## Context

`settings.yaml` decoded with `KnownFields`, so a key the `Settings` struct did
not declare was a hard error, and `main.go` made it fatal before the in-app
updater existed. A newer build that adds a field writes it on its next save; a
config directory synced between machines then lands that file on every machine
still on an older build, and each one needs a binary installed by hand (#482).
Migrations do not cover it: they run forward only, and an additive field does
not bump the version.

## Decision

**A key the build does not declare loads and is ignored.** After the lenient
decode succeeds, a strict decode into a throwaway value reports the unknown
keys, and startup logs them once as a warning. The constraint is that the
updater must be reachable from a file this build cannot fully parse: it is
the only route off a bad version that needs no person at a terminal.

**A type mismatch is still an error**, and a file whose `version` is above the
build's `Current` is still rejected. A version bump is a declared breaking
change; this decision covers the additive case.

**An older build's save drops the keys it does not know.** Keeping them needs
a reflection walk of the schema and a node-level merge on save, and the loss
it prevents is one setting reverting to its default on the newer machine for
as long as the older one stays behind -- which the reachable updater ends.

## Consequences

- A misspelled key no longer fails startup. It is logged and does nothing;
  the `hive-settings` skill prompt says so.
- A new field needs no version bump to stay loadable by older builds. A rename
  or a removal still does.
- Schema validation for editors and a doctor check belongs to a published
  schema, not the loader.
- Flow, actions, and agent-workspace files keep their strict decode; their
  last-good reload already makes a bad file non-fatal.
