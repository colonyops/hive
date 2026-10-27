# Quality gates enforced by lefthook git hooks

- **Status:** accepted
- **Date:** 2026-07-23

## Context

Most commits here come from headless agent sessions with no human watching the terminal, so opt-in gate
tasks are not enforcement. Broken or unformatted commits would otherwise surface only in CI, a push and
several minutes later.

## Decision

`lefthook.yml` defines pre-commit and pre-push hooks. Every job shells out to a mise task, so a gate has
one definition shared by the hook and CI. The split is by cost, because a slow pre-commit stalls agent
loops:

- **pre-commit**: `mise run fmt:staged` on staged Go files, and `mise run check:generate` only when a
  generator input is staged.
- **pre-push**: `mise run check`, plus the frontend test, format, and lint tasks when the push touches
  the matching frontend paths. Push is the wrap/PR boundary for headless work, so the full Go gate
  belongs here.

The Go gate is the single `check` task, called by the hook and CI alike. `check:generate` runs first,
alone, because it rewrites `.go` files the other steps read; the rest run after it. The order is a `run`
list because mise executes `depends` in parallel.

Formatting auto-fixes and re-stages instead of failing, since a blocked commit costs an agent a turn. A
partially staged file gets its unstaged hunks staged along with the formatting.

Hooks install from mise's `postinstall` hook (`setup` runs `scripts/hooks/install.sh`), so `mise install`
is the only setup step. The installer pins the clone's local `core.hooksPath` to its own `.git/hooks`,
because lefthook refuses to install while any `core.hooksPath` is set.

## Consequences

- The `check:*` tasks regenerate or diff and fail with the name of the task that fixes the problem
  (`generate`, `tidy`); they do not leave unrequested rewrites behind.
- Gates too slow for a hook (Wails TS bindings, e2e, `-race`, deadcode, vuln) run only in CI.
- A global `core.hooksPath` is overridden in this repo; global hooks do not run here.
- After editing `lefthook.yml`, run `mise run setup` to re-sync the hooks.
- `LEFTHOOK=0` and `--no-verify` exist for human emergencies; agents must not use them.
