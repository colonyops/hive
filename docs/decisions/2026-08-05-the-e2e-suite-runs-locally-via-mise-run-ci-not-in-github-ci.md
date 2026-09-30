# The e2e suite runs locally via mise run ci, not in GitHub CI

- **Status:** accepted
- **Date:** 2026-08-05

## Context

The Playwright e2e suite was removed from `.github/workflows/ci.yml` because it
was too slow to sit on every PR: it builds a container image and drives four
server instances across three browser projects.

Removing it from CI left it with no gate at all. `mise run ci` claimed to run
"every gate CI runs", which by construction excluded a suite CI no longer ran,
and the hook documentation said "CI covers them either way" — which had stopped
being true. Nothing was left that ran e2e except a person remembering to.

The cost of that showed up as thirteen failing specs. None were regressions:
the detail pane stopped rendering a branch footer, the connector registry grew
Grafana and exec, notification delivery became a select instead of radios,
onboarding gained a notification-permission step, and the starter action
catalog grew a header. Every one was an intended product change whose e2e
expectations were never updated, because nothing failed when they weren't.

## Decision

`mise run ci` ends with `e2e`, as its own final group. It is the gate;
GitHub CI is not.

The task's description and the quality-gates documentation say so explicitly,
rather than describing `ci` as a mirror of the workflow file.

## Consequences

`mise run ci` no longer finishes in ~20s — everything before e2e does, and then
e2e takes minutes. It is last and alone so a cheap failure never waits behind
it, and it needs Docker, so `mise run ci` now requires Docker on a fresh clone.

Because CI does not run e2e, a branch can merge with it broken. The only
defence is running `mise run ci` before opening a PR, so treat skipping it as
skipping the suite entirely rather than as saving a few minutes.

If e2e is ever fast enough to return to CI, this decision is what to revisit —
and the `ci` task and the quality-gates section both have to change with it.
