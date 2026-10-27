# A store owns one entity's persistence and a service coordinates over stores

- **Status:** accepted
- **Date:** 2026-09-05

## Context

A single database object with entity-prefixed methods for every table gives
no type an entity name, leaks generated sqlc rows to callers, and leaves no
rule for where a multi-table write goes.

## Decision

A store owns one aggregate's persistence. It is built from `*queries.DB`,
takes `context.Context` first on every method, returns hand-written domain
types, and publishes no events. A service coordinates stores and other
services, maps store errors to `app.Error` kinds, and publishes events. Only
services hang off `App`; the e2e harness's `App.Stores` and
`App.PipelineDB()` are the exceptions.

The desktop's persistence lives in `cmd/desktop/internal/app/data/`:
`models` holds domain types with no database dependency, `queries` holds the
sqlc output, the DB handle and migrations, and `stores` holds aggregate
persistence. The placement rules and the transaction contract are in
`docs/architecture.md`, "Stores and services".

The constraint behind the transaction contract: the database opens with
`_txlock=immediate` on a two-connection pool, so a store that opened a
second transaction would stall behind its own caller. Every store call goes
through `.Ctx(ctx)` to join an ambient transaction, and a service that spans
aggregates opens `Stores.WithinTx`. `Compact` is the exception, because
SQLite cannot run `VACUUM` inside a transaction.

## Consequences

- A multi-table write belongs to its owning store or a service, so no third
  category of store grows around cross-table work.
- Generated sqlc types stay behind the store boundary in production code.
  `stores.Seed` is the one test-only seam through it.
