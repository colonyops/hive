# Vendor hive `internal/` packages via sync tool

- **Status:** accepted
- **Date:** 2026-07-23

## Context

The desktop app imports ~12 packages from hive's `internal/` tree (core config, session, messaging, git, eventbus, the data layer, the `internal/hive` app façade, GitHub client). Go forbids importing `internal/` packages across module boundaries, so once the desktop lives in its own module it cannot compile against hive's internals at all. The transitive closure is large — most of `internal/core/**`, `internal/data/**`, `internal/sources/**`.

## Decision

Copy the required hive `internal/` closure into `internal/hivecore/` via an automated sync tool (`cmd/vendorhive`), with imports rewritten to this module's path.

- `cmd/vendorhive/vendor.lock` pins the hive commit; prefer pinning to tagged hive releases.
- The tool computes the package closure automatically (`go list -deps` over the seed imports in a clone of hive at the pinned SHA), so the vendored surface shrinks as coupling shrinks.
- Hive's `pkg/**` packages are importable cross-module and stay a normal `go.mod` dependency, pinned to the same SHA by the tool.
- **Vendored code is read-only.** Changes land in `colonyops/hive` first, then re-vendor. CI runs the tool at the pinned SHA and fails on drift.
- Vendored tests are copied and run — free regression coverage of the core on this repo's toolchain.

Rejected alternative: promoting the closure to `pkg/` in hive. Zero tooling and one source of truth, but it publishes hive's entire engine as the public API of a public repo, and every desktop-driven core change becomes a public-surface question. Revisit if vendor churn becomes painful.

## Consequences

- The desktop repo is self-sufficient and private; hive keeps its `internal/` boundary.
- Every hive core change the desktop needs is a two-step: land in hive, re-vendor here.
- **Sharpest risk — shared database schema drift:** the desktop opens the user's live hive SQLite database using vendored `internal/data/*` code, so the installed hive CLI and this repo's vendored snapshot can disagree on schema. Mitigations: pin vendors to tagged hive releases; add a schema-version guard at desktop bootstrap (refuse or go read-only, with a clear message, when the DB's migration version is ahead of the vendored one); make re-vendoring part of the release checklist.
