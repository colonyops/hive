# Product monorepo structure and clean import from hive

- **Status:** accepted
- **Date:** 2026-07-23

## Context

The Hive desktop app is being extracted from the public `colonyops/hive` repo into this private repo. Beyond the desktop app, the product needs an admin backend (analytics, licenses, purchases) and a landing page. We want one repo for the whole product.

## Decision

Private monorepo `hay-kot/hive-desktop` with three components:

- `desktop/` + `internal/app/` + `internal/adapter/` — the Wails v3 app, imported from `colonyops/hive`.
- `server/` — future Go admin backend.
- `web/` — landing page, plain static HTML.

**Module layout:** the root Go module `github.com/hay-kot/hive-desktop` owns the desktop app and vendored hive core. When the server is built, it becomes a nested module (`server/go.mod`) so the deployed service does not carry wails/charm dependencies in its go.sum; a root `go.work` is added at that point. Shared wire types (analytics events, license payloads) would live in a third nested `shared/` module.

**History:** the desktop code arrives as a single clean import commit (`Import desktop from colonyops/hive@<sha>`) — no `git filter-repo` history migration. Full history stays in `colonyops/hive` for archaeology; the recorded SHA doubles as the first vendor pin.

## Consequences

- One repo, one issue tracker, one release cadence to reason about for the whole product.
- `git blame` inside imported code bottoms out at the import commit; deeper archaeology requires the hive repo.
- The module path bakes in the `hay-kot` owner; moving the repo later means a module rename.
