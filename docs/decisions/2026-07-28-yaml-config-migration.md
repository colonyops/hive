# Forward-only, in-place YAML config migration

- **Status:** accepted
- **Date:** 2026-07-28

## Context

Users keep real config in the desktop's YAML files. `flows/*.yaml` and
`actions.yml` decode strictly (`KnownFields(true)`), so a renamed or removed
key is a hard decode error, and `settings.yaml` warns about an unknown key on
every launch. A breaking schema change needs a path forward that neither
fails startup nor silently drops a flow.

## Decision

**Each config file type has its own forward-only, integer-versioned migration
chain, anchored on the file's own top-level `version:` field.** The chains
are `Set`s in `cmd/desktop/internal/app/configmigrate/sets.go` (settings,
flow, actions, agent-workspace, and the workspace MCP and skill libraries).

- Migration runs on raw bytes before the typed decoders: a lax
  `map[string]any` decode, ordered steps up to the set's `Current`, then
  re-marshal and hand off to the normal loader.
- **The load path migrates in memory and never writes.** Only the up-front
  pass in `cmd/desktop/main.go` writes, before any store or watcher exists,
  so a migration write cannot race a watcher reload. A settings failure there
  stops startup; flows, actions and workspaces fall back to last-good with a
  warning.
- Before each atomic rewrite, the pre-migration bytes are backed up under
  `<StateDir>/migration-backups/`. If the backup fails, the rewrite is
  aborted.
- A `version:` above the build's `Current` is `ErrVersionTooNew`. There is no
  downgrade.
- A migration step is frozen against old data: it must not read live
  registries or presets, so the same old file always migrates to the same
  result.

## Consequences

- A migrating rewrite drops comments, formatting and key order for good,
  because it re-marshals a generic map. The comment-preserving normal saves
  (`flow.SaveFlow`, the actions store) then have nothing left to preserve.
  A file that never migrates is unaffected.
- Backups accumulate with no GC and no UI.
- Each file type versions independently.
- An old file introduced while the app runs migrates in memory at once but is
  rewritten on disk only at the next launch.
- A breaking change is a `Current` bump plus one `Migration` step.
