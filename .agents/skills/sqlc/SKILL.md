---
name: sqlc
description: Add SQL queries, run code generation, and configure type overrides for the hive SQLite data layer. Use when writing new queries, adding tables, or mapping domain types to generated code.
compatibility: claude
---

# sqlc in Hive

Hive uses [sqlc](https://sqlc.dev) to generate type-safe Go from SQL queries. The generated files are committed to the repo — never edit them manually.

## File Layout

```
internal/data/db/
├── queries/
│   ├── queries.sql          # Core session/message queries
│   └── queries_hc.sql       # Honeycomb queries (separate file)
├── migrations/
│   └── NNNN_name.up.sql     # Schema migrations (source of truth for sqlc)
├── queries.sql.go           # Generated — do not edit
├── queries_hc.sql.go        # Generated — do not edit
└── models.go                # Generated — do not edit
```

`sqlc.yaml` at the repo root lists both query files under `sql[0].queries`.

## Running Code Generation

After any change to `.sql` query files or `sqlc.yaml`:

```bash
mise run generate    # runs sqlc generate + go-enum generation
# or directly:
sqlc generate
```

Always commit the generated `*.sql.go` and `models.go` alongside the SQL changes.

## Adding a Query

1. Write the annotated query in the appropriate `.sql` file:

```sql
-- name: GetHCItem :one
SELECT id, repo_key, epic_id, parent_id, session_id, title, "desc",
       type, status, depth, created_at, updated_at
FROM hc_items WHERE id = ?;

-- name: ListHCItems :many
SELECT ... FROM hc_items WHERE repo_key = ? ORDER BY created_at DESC;

-- name: InsertHCItem :exec
INSERT INTO hc_items (...) VALUES (...);
```

Annotations: `:one` (returns single row), `:many` (returns slice), `:exec` (no rows returned).

2. Run `mise run generate`.

3. The generated function appears in `queries_hc.sql.go` (or `queries.sql.go`) under the `db` package.

4. Call it via `s.db.Queries().GetHCItem(ctx, id)`.

## Type Overrides (Domain Types in Generated Code)

When a column stores a domain enum, add an override in `sqlc.yaml` so the generated params/returns use the Go type directly instead of `string`:

```yaml
overrides:
  - column: "hc_items.status"
    go_type:
      import: "github.com/colonyops/hive/internal/core/hc"
      type: "Status"
  - column: "hc_items.type"
    go_type:
      import: "github.com/colonyops/hive/internal/core/hc"
      type: "ItemType"
```

The domain type must implement `driver.Valuer` and `sql.Scanner` (or use text marshaling). go-enum generated types satisfy this via `MarshalText`/`UnmarshalText`, which SQLite handles as text.

## Schema Source of Truth

sqlc derives the schema from `internal/data/db/migrations/*.up.sql`. When you add a migration, run `mise run generate` to regenerate models. The generated `models.go` is always overwritten — do not add hand-written code there.

## Separate Query Files

HC queries live in `queries_hc.sql` to keep the diff surface small. Add new feature query files by listing them under `sql[0].queries` in `sqlc.yaml`. All query files share the same `gen.go` output directory and package.

## Common Patterns

**Nullable / optional columns:** Use `sql.NullString`, `sql.NullInt64`, etc. Override with `nullable: true` in `sqlc.yaml` if needed.

**Timestamps:** Stored as `INTEGER NOT NULL` (Unix seconds). The generated code uses `int64`. Conversion to/from `time.Time` happens in the store layer, not in generated code.

**Transactions:** Use `s.db.WithTx(ctx, func(q *db.Queries) error { ... })` — the `db.DB` wrapper provides this. Queries inside the closure use the transactional `Queries` instance.
