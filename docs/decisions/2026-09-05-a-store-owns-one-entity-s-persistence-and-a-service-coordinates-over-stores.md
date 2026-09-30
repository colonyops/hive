# A store owns one entity's persistence and a service coordinates over stores

- **Status:** proposed
- **Date:** 2026-09-05

## Context

One `*store.DB` holds 71 entity-prefixed methods over 14 tables. No type carries
an entity name, so every method repeats it. Services either hold the whole
object or define a narrow interface over it. `db.Queries()` leaks sqlc rows to
seven production sites.

`activity.Store` and `jobs.Store` already own one entity's persistence over the
same database. The vendored `internal/hivecore/data/stores/` does the same for
seven entities.

## Decision

A store owns one aggregate's persistence. It is built from `*queries.DB`, takes
`context.Context` first on every method, returns hand-written domain types, and
publishes no events. A service coordinates stores and other services, maps
store errors to `app.Error` kinds, and publishes events. Only services hang off
`App`; the e2e harness's `App.Stores` and `App.PipelineDB()` are the
exceptions.

The placement rules, the four clauses and the transaction contract, live in
`docs/architecture.md` under "Stores and services". The constraint that forced
them: the database runs `_txlock=immediate` on a two-connection pool, so a
store that opened a second transaction would stall behind its own caller.
Every store call therefore goes through `.Ctx(ctx)`, and a service that spans
aggregates opens `Stores.WithinTx`. `Compact` is the exception because SQLite
cannot run `VACUUM` inside a transaction.

Persistence moves to `internal/app/data/{models,queries,stores}`. `models`
holds domain types without a database dependency, `queries` holds sqlc output,
the DB handle, and migrations, and `stores` holds aggregate persistence.

## Consequences

`internal/app/store` disappears as a name. A multi-table write belongs to its
owning store or a service, so no third store category grows around cross-table
work. The generated package stays behind the store boundary in production
code; `stores.Seed` is the one test-only seam through it, so a test outside
`data/` builds a fixture from store types.
