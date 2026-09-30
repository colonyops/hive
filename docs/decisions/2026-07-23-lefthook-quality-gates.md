# Quality gates enforced by lefthook git hooks

- **Status:** accepted
- **Date:** 2026-07-23

## Context

The quality gates (`fmt`, `lint`, `test`, generated-file and vendor drift checks) existed only as opt-in mise tasks. Most commits here come from headless agent sessions with no human watching the terminal, so "remember to run the gates" is not an enforcement mechanism: unformatted or broken commits land, and CI catches them a push and several minutes later.

## Decision

`lefthook.yml` defines pre-commit and pre-push hooks. Every job shells out to a mise task, so a gate has exactly one definition shared by the hook and by CI.

The split is by cost, because a slow pre-commit stalls agent loops:

- **pre-commit** (~0.1s): `golangci-lint fmt` on staged Go files, a guard against editing vendored `internal/hivecore/`, and a generated-code drift check that only fires when a generator input is staged.
- **pre-push** (~2s, ~6s with the frontend): `mise run check` — `check:generate`, `check:tidy`, `lint`, `test` — and the frontend unit tests when the push touches `desktop/frontend/`. Push is the wrap/PR boundary for headless work, so the full gates belong here.

The Go gate is one `check` task rather than one lefthook job per step, so the sequence has a single definition that the hook, CI, and the release workflow all call instead of restating. Its steps run ordered rather than in parallel: `check:generate` rewrites the same `.go` files that `lint` and `test` read, and a torn read would surface as an unreproducible failure — worth more than the seconds parallelism would save. The order is expressed as a `run` list because mise executes `depends` in parallel, which would reintroduce exactly that race.

Frontend tests stay a separate lefthook job rather than joining `check`: they are only worth running when the push touches `desktop/frontend/`, and that condition belongs in the hook rather than in a gate that CI and the release workflow also call.

Formatting **auto-fixes and re-stages** rather than failing: a blocked commit costs an agent a whole turn to re-run and re-commit for whitespace. The trade-off is that a partially staged file gets its unstaged hunks staged along with the formatting.

Hooks install from mise's `postinstall` hook (`scripts/hooks/install.sh`), so `mise install` on a fresh clone is the only setup step. The installer pins the clone's local `core.hooksPath` to its own `.git/hooks`: lefthook refuses to install while a `core.hooksPath` is set (a common global dotfiles setting), and pinning it locally means the forced install can only ever write into this repo.

## Consequences

- Unformatted Go, hand-edited vendored code, stale generated output, an untidy `go.mod`, and failing tests are all caught locally within seconds of being written.
- `mise run check` is the whole Go gate as one command, so anything that wants to run "what runs before a push" calls it rather than listing steps that then rot. Adding a gate means editing one task.
- The `check:*` tasks regenerate in place and diff, so a failure leaves the corrected output in the working tree to inspect and a pass leaves file contents as they were. What they no longer do is rewrite something nobody asked about: the unconditional `go mod tidy` that `check` used to run is now its own `tidy` task, and each failing check names the task that fixes it.
- CI stays the source of truth. Gates too slow for a hook — Wails TS binding drift, the Docker/Playwright e2e suite, `-race`, and the network-dependent `check:vendor` — run only there.
- A global `core.hooksPath` is overridden in this repo; global hooks do not run here.
- `lefthook.yml` changes need `mise run setup` to re-sync the hook files, since lefthook's auto-sync declines to run while `core.hooksPath` is set.
- `LEFTHOOK=0` / `--no-verify` remain available for human emergencies; AGENTS.md forbids agents from using them.
