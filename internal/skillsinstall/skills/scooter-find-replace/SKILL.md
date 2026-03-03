---
name: scooter-find-replace
description: Use the scooter CLI for find and replace operations across codebases. Use when the user asks for bulk find-and-replace, renaming symbols across files, codebase-wide text substitution, or when sed/awk would be too fragile for multi-file replacements.
---

# Scooter Find and Replace

scooter is an interactive find-and-replace terminal tool. It respects `.gitignore` and `.ignore` files, supports regex with capture groups, and can run in both TUI and non-interactive modes.

## When to Use Scooter

Prefer scooter over `sed`, `awk`, or manual multi-file edits when:
- Replacing text across many files at once
- The user wants interactive review of replacements before applying
- Regex capture groups are needed in replacements
- File filtering by glob pattern is required

For single-file, single-location edits, use the StrReplace tool instead.

## Non-Interactive Mode (Agent Use)

Since agents cannot operate TUIs, use `--no-tui` (`-N`) for automated replacements:

```bash
scooter -N -s "search_pattern" -r "replacement" [directory]
```

The `-N` flag combines immediate search, immediate replace, and stdout output with no UI.

### Common Flags

| Flag | Short | Purpose |
|------|-------|---------|
| `--search-text` | `-s` | Text/regex to search for |
| `--replace-text` | `-r` | Replacement text |
| `--fixed-strings` | `-f` | Plain string match (not regex) |
| `--case-insensitive` | `-i` | Ignore case |
| `--match-whole-word` | `-w` | Match whole words only |
| `--files-to-include` | `-I` | Comma-separated glob patterns to include |
| `--files-to-exclude` | `-E` | Comma-separated glob patterns to exclude |
| `--hidden` | `-.` | Include hidden files/directories |
| `--advanced-regex` | `-a` | Enable lookahead and other advanced regex features |
| `--no-tui` | `-N` | Fully non-interactive (search + replace + print) |
| `--immediate-search` | `-S` | Skip search screen, jump to results |
| `--immediate-replace` | `-R` | Replace without confirmation |
| `--immediate` | `-X` | Combines `-S`, `-R`, and print results |
| `--print-results` | `-P` | Print results to stdout |

## Examples

### Simple string replacement across codebase

```bash
scooter -N -f -s "oldFunctionName" -r "newFunctionName"
```

### Regex with capture groups

Capture groups in the search are referenced with `$1`, `$2`, etc. in the replacement:

```bash
scooter -N -s '(\w+)\.toFixed\(2\)' -r 'formatCurrency($1)'
```

### Scoped to specific file types

```bash
scooter -N -f -s "old_name" -r "new_name" -I "*.go,*.ts"
```

### Exclude directories

```bash
scooter -N -f -s "TODO" -r "FIXME" -E "vendor/**,node_modules/**"
```

### Case-insensitive whole-word replacement

```bash
scooter -N -f -i -w -s "colour" -r "color"
```

### Scoped to a specific directory

```bash
scooter -N -f -s "old_api" -r "new_api" src/services/
```

### Stdin piping

```bash
cat input.txt | scooter -N -s "before" -r "after" > output.txt
```

## Interactive Mode (User-Driven)

When the user wants to review replacements interactively, suggest the TUI mode:

```bash
# Launch with pre-populated fields
scooter -s "pattern" -r "replacement"

# Launch with pre-populated fields and skip to results
scooter -S -s "pattern" -r "replacement"
```

In TUI mode, the user can toggle individual replacements on/off with `space`, toggle all with `a`, and press `enter` to apply.

## Key Behaviors

- Respects `.gitignore` and `.ignore` automatically
- Glob patterns follow ripgrep conventions (e.g., `dir1/**` not just `dir1`)
- If a matched line has changed since the search (e.g., branch switch), that replacement is safely skipped
- By default uses a fast regex engine; use `-a` for advanced features like negative lookahead

## Safety

- Always do a dry run first when unsure: use `scooter -X -s "pattern" -r "replacement"` to see what would change (prints to stdout without `-N` replacing)
- For destructive or wide-reaching replacements, prefer the interactive TUI so the user can review each match
- Suggest `git diff` after replacements to verify changes
