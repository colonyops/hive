# The e2e suite runs locally via mise run ci, not in GitHub CI

- **Status:** superseded by [the-full-desktop-e2e-suite-runs-in-github-ci](2026-10-23-the-full-desktop-e2e-suite-runs-in-github-ci.md)
- **Date:** 2026-08-05

## Context

The desktop Playwright e2e suite is too slow to run on every PR: it builds a
container image and drives several server instances across browser projects.
Taking it out of GitHub CI left it with no gate, and intended product changes
broke its expectations without anything failing.

## Decision

`mise run ci` ends with `desktop:e2e`, as its own final group. That task is the
gate; GitHub CI does not run e2e. The task description and the quality-gates
docs say so.

## Consequences

- `mise run ci` takes minutes and needs Docker. E2e runs last and alone so a
  cheap failure never waits behind it.
- A branch can merge with e2e broken. Running `mise run ci` before opening a PR
  is the only defence.
- If e2e moves back into GitHub CI, the `ci` task and the quality-gates docs
  change with it.
