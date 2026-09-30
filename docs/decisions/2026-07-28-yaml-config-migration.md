# Forward-only, in-place YAML config migration

- **Status:** accepted
- **Date:** 2026-07-28

## Context

ADR desktop-configuration's no-migration stance for `settings.yaml` was a pre-alpha choice:
before users had configs worth preserving, a breaking schema change was
allowed to simply break old files. That is no longer true for `settings.yaml`,
`flows/*.yaml`, and `actions.yml` — all three decode strictly
(`KnownFields(true)`), so a renamed or removed key is a hard decode error, and
users now accumulate real config in these files. A breaking change needs a
path forward that does not hard-fail startup or silently drop a flow.

## Decision

**Each of the three config files gets its own forward-only, integer-versioned
migration chain, anchored on the file's own top-level `version:` field** (not a
tracking table), modeled on the SQLite runner
(`internal/hivecore/data/migrate`) but per-file rather than per-database.

- Migration runs on raw bytes, before the strict decoders: a lax
  `map[string]any` decode, ordered transforms up to the build's current
  version, then re-marshal and hand off to the existing strict loader.
- **Two entry points, split by responsibility.** The load path
  (`loadSettingsAt`, `LoadFlow`, `LoadActions`) applies migration purely in
  memory and never writes. Persistence happens only in the up-front startup
  pass (`desktop/main.go`), which runs before any store or watcher exists and
  is the sole writer — so no migration write can race a watcher-triggered
  reload.
- A timestamped backup of the pre-migration bytes is written under
  `<StateDir>/migration-backups/` before each atomic rewrite; if the backup
  write fails, the rewrite is aborted and the source file is left untouched.
- A file whose `version:` exceeds the build's current version is a hard error
  (`ErrVersionTooNew`) — forward-only, no downgrade path.
- The initial ship is inert: all three Sets start at `Baseline == Current ==
  1`, so nothing is rewritten until a real breaking change bumps a `Current`
  and adds a migration step.

## Consequences

- **Comment loss is permanent on the first migrating rewrite, not a one-time
  cosmetic hit.** Because a migration re-marshals a `map[string]any`,
  `yaml.v3` sorts keys and drops comments; once a file has gone through one
  migrating rewrite, its comments and formatting are gone for good, and every
  comment-preserving normal save afterward (`flow.SaveFlow`, the actions store
  CRUD) has nothing left to preserve. A never-migrated file's normal saves are
  unaffected.
- Migration backups under `<StateDir>/migration-backups/` accumulate with no
  GC and no UI; they are local recovery state, not synced config.
- Each file type has an independent version sequence — bumping `Current` for
  flows does not require touching settings or actions.
- A runtime reload of an externally introduced old file (e.g. hand-edited
  while the app is running) migrates in memory immediately but is only
  rewritten on disk at the next launch's up-front pass.
- Adding the next breaking change is a two-line addition — bump a Set's
  `Current` and append the corresponding `Migration` step — not a new
  subsystem.
- This amends ADR desktop-configuration's no-migration stance for config **files**; see the
  update note there. It does not touch environment variable naming, which
  ADR desktop-configuration still governs unchanged.
