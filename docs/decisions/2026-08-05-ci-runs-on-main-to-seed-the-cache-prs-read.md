# CI runs on main to seed the cache PRs read

- **Status:** accepted
- **Date:** 2026-08-05

## Context

PR CI took ~18 minutes on most pull requests and ~4.5 minutes on a few, with nothing in the diff to explain which one you got. The `go` job was bimodal: ~250s or ~1080s, with every expensive step scaling together — `Set up mise` 19s or 131s, `golangci-lint` 20s or 285s, `go test -race` 97s or 474s.

The split is cache warmth, and the trigger was the cause. GitHub scopes each cache entry to the ref that wrote it. A run may restore from its own scope or from the default branch's, and from nowhere else. `ci.yml` triggered on `pull_request` alone, so every entry it wrote landed in `refs/pull/N/merge` — unreadable by any other PR, and by the same PR after a rebase. Nothing had ever written to main's scope, so a new PR had nothing to restore and rebuilt the Go build cache, the mise toolchain, and the golangci-lint analysis cache from nothing. The fast runs were second-and-later pushes to a PR that had already paid for its own cache.

That also overflowed the cache budget. Eleven per-PR copies of the ~905MB `setup-go` entry made 9.07GB of a 12.83GB total against GitHub's 10GB limit, so LRU eviction was constantly discarding entries that were still wanted — which is why even a repeat push on the same branch sometimes came back cold.

Within a run, the `go` job was also one serial chain of eight steps that do not depend on each other.

## Decision

CI runs on pushes to `main` as well as on pull requests. The run exists to leave a cache in the default branch's scope; post-merge signal is a side effect, not the reason.

`cancel-in-progress` is now conditional on the event being a `pull_request`. A cancelled main run writes no cache, which would defeat the trigger; superseded PR runs are still waste worth cancelling.

The `go` job is split in two. `go` keeps the drift checks, the build, and the race-detector tests; `go-quality` takes `golangci-lint`, `check:deadcode`, and `check:vuln`. `golangci-lint` runs first in `go-quality` because it is the only one of the three that reports a compile error legibly. `desktop-frontend` splits the same way, into a `vue-tsc`/vite build job and a vitest job.

`-race` moves off the PR path and onto the main run. Profiling put the whole suite at 49s of sequential execution (11.8s of it `cmd/release`'s disk-image test, which skips on Linux), against a 97s CI step — the gap is race-instrumented compilation and linking of 81 test binaries, and dropping the flag took a CI-shaped local run from 43.1s to 17.2s. Merges to main still run `test:race`, so a data race is caught before a release rather than before a review. The two invocations are `mise run test` and `mise run test:race` rather than inline flags, so the workflow and the local task cannot disagree about what "the tests" means.

Two tests that waited on real timers became `testing/synctest` bubbles: the tmuxcc broker/manager pump-teardown pair and the mock GitHub device flow. That is worth ~2s and was not the reason for the change — the reason is that `synctest.Wait()` plus a fake-clock `time.Sleep(finalDelivery)` asserts the pump exits *at* its bounded window, where `require.Eventually` could only poll a 5s ceiling and pass for the wrong reason. The suite's other real-time waits are not eligible: `execenv` spawns login shells and the `agentws`/`actions` watchers take real fsnotify events, neither of which synctest can bubble.

A `mise run ci` task runs every gate the workflow runs, so an agent can answer "will CI pass?" in ~20s locally instead of pushing and waiting minutes. It is the superset `check` does not cover — bindings, vendor drift, deadcode, govulncheck, and the frontend — and is deliberately not a git hook, because it needs the network and a built frontend.

Package installation stays uncached. `libgtk-4-dev` and `libwebkitgtk-6.0-dev` pull a dependency tree with `ldconfig` triggers and postinstall scripts, and a subtly incomplete restore surfaces as an unrelated-looking cgo failure. ~30s per job is not worth buying with that failure mode.

## Consequences

- A PR's first run restores from main instead of building from nothing. The cold ~1080s path is gone in the normal case; wall clock is bounded by the `go` job at roughly 200s.
- Cache pressure drops out on its own. `setup-go` writes only on a key miss, so PRs that match main's entry stop saving a 905MB copy each, and the total falls back under the limit rather than thrashing.
- Every merge to main costs a full CI run. That is the price of the warm cache, and it buys post-merge signal that `pull_request`-only CI did not have.
- Four jobs now pay toolchain setup where two did. Machine time goes up; wall clock, which is what a PR waits on, goes down.
- A cache-relevant change — the Go toolchain, `go.sum`, the mise tool pins — still costs one cold run, now on main rather than on whichever PR happened to land first.
- Rerunning a stale PR against a much newer main can restore a cache built from a distant tree. Go's content-addressed build cache makes that a partial hit rather than a wrong one.
- A data race now surfaces on main rather than in review. The window is one merge, and `mise run test:race` reproduces it locally, but it is a real reduction in what a PR proves.
- `mise run ci` and `.github/workflows/ci.yml` are two lists that have to agree. The task calls the same mise tasks the workflow does, so a gate added as a task lands in both; a gate added as an inline `run:` step in the workflow lands in neither.
