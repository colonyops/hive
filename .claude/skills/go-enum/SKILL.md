---
name: go-enum
description: Define and extend Go enums using the go-enum code generator. Use when adding new enum types, extending existing ones, or running generation after changes.
compatibility: claude
---

# go-enum in Hive

Hive uses [go-enum](https://github.com/abice/go-enum) to generate enum methods from `// ENUM(...)` comments. Generated files are committed — never edit them manually.

## How It Works

Define an enum type with a special comment:

```go
// ItemType categorizes an HC item.
//
// ENUM(epic, task)
type ItemType string
```

go-enum reads the comment and generates `item_enum.go` (or the file named after the type's source file with `_enum` suffix) containing:

- `ParseItemType(s string) (ItemType, error)`
- `(t ItemType) IsValid() bool`
- `(t ItemType) String() string`
- `(t ItemType) MarshalText() ([]byte, error)` / `UnmarshalText([]byte) error`
- `ItemTypeValues() []ItemType`
- Constants: `ItemTypeEpic`, `ItemTypeTask`

## Running Generation

After any change to an `// ENUM(...)` comment or adding a new enum type:

```bash
mise run generate:enums    # go-enum only
mise run generate          # all generators (go-enum + sqlc)
```

## Adding a New Enum Value

1. Add the value to the `// ENUM(...)` comment:

```go
// ENUM(epic, task, milestone)
type ItemType string
```

2. Run `mise run generate:enums`.

3. The new constant `ItemTypeMillestone` and `ParseItemType("milestone")` are available.

4. Update any `switch` statements or `OneOf` validators that enumerate values.

## Adding a New Enum Type

1. Create the type with the ENUM comment in the appropriate source file:

```go
// Priority ranks the urgency of an item.
//
// ENUM(low, medium, high, critical)
type Priority string
```

2. Run `mise run generate:enums`. A new `*_enum.go` file is generated next to the source.

3. Never define the constants manually — the generator owns them.

## Generated File Naming

go-enum creates files named `<source_file_stem>_enum.go`. Examples:
- `item.go` → `item_enum.go`
- `activity.go` → `activity_enum.go`

If a source file defines multiple enum types, all are generated into the same `_enum.go` file.

## Integration with sqlc

When a column stores an enum, the generated go-enum type satisfies `driver.Valuer`/`sql.Scanner` via its `MarshalText`/`UnmarshalText` methods. Add an override to `sqlc.yaml`:

```yaml
overrides:
  - column: "hc_items.type"
    go_type:
      import: "github.com/colonyops/hive/internal/core/hc"
      type: "ItemType"
```

This lets sqlc use `hc.ItemType` directly in generated query params/returns — no string conversion needed in the store layer.

## Integration with criterio Validation

Use `criterio.OneOf(...)` to validate enum fields, listing all valid constants explicitly:

```go
criterio.Run("type", i.Type, criterio.OneOf(ItemTypeEpic, ItemTypeTask))
```

Do not call `i.Type.IsValid()` inside criterio — `OneOf` is more readable and produces better error messages.

## Gotchas

- **Never edit `*_enum.go` files** — they are overwritten on every generation run.
- **Zero value is not a valid enum.** An unset `ItemType("")` will fail `IsValid()` and `OneOf` checks. Always initialize enum fields.
- **String values match the ENUM comment exactly** (lowercase by default). `ItemTypeEpic` has string value `"epic"`.
- **`mise run generate:enums` sources** are listed in `mise.toml` under `[tasks."generate:enums"]`. If you add a new source file containing an ENUM, add it to the `sources` list so mise detects changes correctly.
