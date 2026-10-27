# CI runs on main to seed the cache PRs read

- **Status:** accepted
- **Date:** 2026-08-05

## Context

GitHub scopes each cache entry to the ref that wrote it, and a run can restore
only from its own scope or the default branch's. CI that triggers on
`pull_request` alone writes every entry to `refs/pull/N/merge`, which no other PR
can read, so every new PR rebuilt the Go build cache, the mise toolchain, and
the golangci-lint cache from nothing. The per-PR copies also overran the 10GB
cache limit and evicted entries still in use.

## Decision

- **`ci.yml` runs on pushes to `main` as well as on pull requests.** The main run
  exists to leave a cache in the default branch's scope.
- **`cancel-in-progress` applies only to `pull_request` events.** A cancelled
  main run writes no cache.
- **Independent gates run as parallel jobs**: `go` (drift checks, build, tests),
  `go-lint` (golangci-lint, workflow lint, goreleaser config, deadcode, vuln),
  and the frontend build and test jobs.
- **`-race` runs on main, not on PRs.** PRs run `mise run test`; main runs
  `mise run test:race`. The workflow calls mise tasks, not inline flags, so it
  and the local task agree on what "the tests" means.
- **System packages are not cached.** An incomplete restore of the GTK/WebKit
  dev packages fails as an unrelated-looking cgo error.

## Consequences

- A PR's first run restores from main's cache.
- Every merge to main costs a full CI run, which also gives post-merge signal.
- A cache-relevant change (the Go toolchain, `go.sum`, mise pins) costs one
  cold run, on main.
- A data race surfaces on main, not in review. `mise run test:race` reproduces
  it locally.
- `mise run ci` and `.github/workflows/ci.yml` must agree. Add a gate as a mise
  task, not an inline `run:` step, so both pick it up.
