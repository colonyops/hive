# The full desktop e2e suite runs in GitHub CI

- **Status:** accepted
- **Date:** 2026-10-23

## Context

ADR [the-e2e-suite-runs-locally-via-mise-run-ci-not-in-github-ci](2026-08-05-the-e2e-suite-runs-locally-via-mise-run-ci-not-in-github-ci.md)
kept the Playwright suite out of GitHub CI because it was too slow for every
PR. That left `mise run ci` as the only gate, and a branch could merge with
the suite broken whenever someone skipped it.

The suite now runs 99 tests in about a minute and a half on one worker. Most
of a run is the image build, not the tests.

## Decision

The `desktop-e2e` job in `.github/workflows/ci.yml` runs
`mise run desktop:e2e`, the full suite, on every push to main and on every PR
that touches the desktop, the shared engine, or the frontend workspace. It is
part of `ci-ok`, so a failure blocks the merge.

The job runs the same Docker harness as a local run. There is no separate CI
mode beyond the `CI=1` the image already sets, which gives each test two
retries. A failed job uploads `test-results/` with the traces.

`mise run ci` keeps its e2e group, so a local run still covers what CI does.

## Consequences

A desktop PR waits for an image build plus the suite. If that becomes too
slow, `--grep @critical` selects the smoke and workflow tests
(`cmd/desktop/e2e/AGENTS.md`), so a smaller PR subset is a one-line change.

Retries can hide a flaky test in CI. `PW_FAIL_ON_FLAKY` makes a retried pass
fail the run; use it locally to hunt a flake that CI reports as flaky.
