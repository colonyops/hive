# Agent Instructions

## Project Overview

**Hive** is a CLI/TUI for managing multiple AI agent sessions in isolated git environments. Instead of manually managing worktrees, hive handles cloning, recycling, and spawning terminal sessions with your preferred AI tool.

Key capabilities:

- **Session Management** - Create, recycle, and prune isolated git clones
- **Terminal Integration** - Real-time status monitoring of AI agents in tmux
- **Inter-agent Messaging** - Pub/sub communication between sessions
- **Context Directories** - Shared storage per repository via `.hive` symlinks

## Architecture

### Core Concepts

**Session** - An isolated git environment for an AI agent
  - Isolated git clone in a dedicated directory
  - Lifecycle states: active, recycled, corrupted
  - Unique ID and display name
  - Metadata for terminal integration

**Agent** - The AI tool instance (Claude, Aider, Codex) running within a session
  - Detected via terminal output patterns
  - Status monitoring: Active, Idle, Waiting, Error, Missing
  - Future: Multiple agents per session

**Terminal Integration** - Real-time status monitoring (tmux)
  - Maps hive sessions to tmux sessions
  - Captures terminal output for status detection
  - Shows live preview of agent work

**Messaging** - Inter-agent communication via pub/sub
  - Inbox convention: `agent.<session-id>.inbox`
  - Topics support wildcards for broadcast
  - Persistent message storage

### Code Structure

```
internal/
├── commands/       # CLI command handlers (urfave/cli/v3)
├── core/
│   ├── config/     # Configuration loading, validation, defaults
│   ├── git/        # Git operations (clone, pull, status)
│   └── session/    # Session model and Store interface
├── hive/           # Service layer - orchestrates all operations
├── integration/
│   └── terminal/   # Terminal status monitoring (tmux)
├── store/
│   └── jsonfile/   # JSON file session storage implementation
├── tui/            # Bubble Tea TUI (tree view, modals, keybindings)
├── messaging/      # Pub/sub messaging between agents
├── printer/        # Output formatting utilities
└── styles/         # Shared lipgloss styles
```

### Key Files

| File                                        | Purpose                                             |
| ------------------------------------------- | --------------------------------------------------- |
| `main.go`                                   | CLI entry point, global flags, command registration |
| `internal/hive/service.go`                  | Service layer - coordinates sessions, git, rules    |
| `internal/core/config/config.go`            | Config structs, loading, defaults                   |
| `internal/core/config/validate.go`          | Template data structs, validation                   |
| `internal/tui/model.go`                     | TUI model, update loop, view rendering              |
| `internal/tui/tree_view.go`                 | Session tree with status indicators                 |
| `internal/integration/terminal/detector.go` | AI agent status detection patterns                  |

## Development

### Commands

```bash
mise run start            # Run with global config (supports CLI args)
mise run dev              # Run with dev config (supports CLI args)
mise run dev -- new       # Example: run 'hive new' with dev config
mise run build            # Build with goreleaser
mise run test             # Run tests with go test
mise run lint             # Run golangci-lint
mise run check            # tidy + lint + test (full validation)
mise run coverage         # Generate coverage report
```

### Environment

Dev environment uses `dev/config.dev.yaml` and `.data/` for isolation:

```bash
HIVE_LOG_LEVEL=debug
HIVE_LOG_FILE=./dev.log
HIVE_CONFIG=./dev/config.dev.yaml
HIVE_DATA_DIR=./.data
```

## Code Generation

### go-enum

Enum types use `// ENUM(...)` comments processed by go-enum. Generated files (`*_enum.go`) are committed and must never be edited manually.

```bash
mise run generate:enums    # regenerate after changing ENUM comments
mise run generate          # all generators (go-enum + sqlc)
```

**Defining an enum:**
```go
// ENUM(epic, task)
type ItemType string
```

This generates constants (`ItemTypeEpic`, `ItemTypeTask`), `ParseItemType`, `IsValid`, `MarshalText`/`UnmarshalText`, and `ItemTypeValues`. String values match the ENUM comment exactly (lowercase).

**When adding a new value**, update the `ENUM(...)` comment, run `mise run generate:enums`, then update any `switch` statements or `criterio.OneOf(...)` validators that enumerate the values. Also add the source file to `sources` in `mise.toml` under `[tasks."generate:enums"]` if it's a new file.

### sqlc

Queries live in `internal/data/db/queries/`. Generated files (`queries*.sql.go`, `models.go`) are committed and must never be edited manually.

```bash
mise run generate    # regenerates after SQL or sqlc.yaml changes
sqlc generate        # directly
```

**Adding a query:** write the annotated SQL (`:one`, `:many`, `:exec`), run generation, then call via `s.db.Queries().FunctionName(ctx, ...)`.

**Type overrides:** when a column stores a domain enum, add an override to `sqlc.yaml` so the generated code uses the Go type directly instead of `string`. The go-enum type satisfies the required interfaces via `MarshalText`/`UnmarshalText`.

Always commit the generated `*.sql.go` and `models.go` alongside the SQL changes in the same commit.

## Code Patterns

### Integration Tests

Integration tests live in `test/integration/` and require a compiled binary. They use the `integration` build tag and are excluded from the standard `mise run test` run.

```bash
mise run test -- -tags integration    # run integration tests
```

**Key rules:**
- Every test calls `NewHarness(t)` which creates isolated `dataDir` and `homeDir` per test — no shared state between tests.
- Use `h.RunStdout(...)` (not `h.Run`) when parsing JSON output — it separates stdout from stderr (migration logs etc. go to stderr).
- Use `h.RunJSONLines(...)` to get `[]map[string]any` from JSONL output directly.
- Use `h.RunWithStdin(input, ...)` to pipe JSON to stdin (for bulk create etc.).
- Avoid `h.Run(...)` for structured output; use it only when testing error messages or combined output.
- Tests that need a real hive session (e.g. `hc next`) must create one with `h.CreateSession(t)` first.
- Do not test formatting/cosmetic output — test field values and structural correctness only.

What belongs in integration tests vs unit tests:
- **Integration**: end-to-end CLI flag wiring, stdin/stdout behavior, multi-command workflows, session detection
- **Unit**: business logic, validation rules, store behavior (using real SQLite via `db.Open(t.TempDir(), ...)`), service orchestration

### Bubble Tea (TUI)

Standard Model/Update/View pattern. Key messages:

- `sessionsLoadedMsg` - Sessions fetched from store
- `gitStatusBatchCompleteMsg` - Git status for all sessions
- `terminalPollTickMsg` - Terminal status polling tick
- `actionCompleteMsg` - Keybinding action finished

### Configuration

Two validation phases:

1. **Basic** (`Validate()`) - Struct validation, required fields
2. **Deep** (`ValidateDeep()`) - File access, template syntax, regex patterns

### Templates

Commands support Go templates with `shq` function for shell quoting:

```yaml
spawn:
  - my-script {{ .Name | shq }} {{ .Path | shq }}
```

Available variables vary by context - see `internal/core/config/validate.go` for `*TemplateData` structs.

### Error Handling

Never silently discard errors. If an error cannot be presented to the user (e.g., in background polling, cache refresh, or TUI status fetching), log it at an appropriate level (`debug` for expected/transient failures, `warn` for configuration problems). Prefer degraded behavior with logging over silent fallbacks — for example, show a `StatusMissing` indicator instead of dropping an item from the UI.

### Session States

```
(new) ──► active ──► recycled ──► (deleted)
              │           │
              └──► corrupted ──► (deleted)
```

## Honeycomb (hc) — Multi-Agent Task Coordination

`hive hc` is the built-in task coordination system for multi-agent workflows. A conductor creates epics and tasks; workers claim and complete them.

### Quick Reference

```bash
# Conductor: create work
echo '{"title":"Epic","type":"epic","children":[{"title":"Task","type":"task"}]}' | hive hc create

# Worker: claim next task
hive hc next <epic-id> --assign

# Worker: record progress
hive hc comment <id> "implemented X"
hive hc comment <id> "CHECKPOINT: stopping here, Y still needed"

# Worker: complete task
hive hc update <id> --status done

# Get context for an epic (markdown for AI consumption)
hive hc context <epic-id>

# List tasks
hive hc list                          # all items
hive hc list <epic-id>                # items under an epic
hive hc list --status open            # filter by status
hive hc list --session <session-id>   # filter by session
```

### Key Commands

| Command | Purpose |
| ------- | ------- |
| `hive hc create [title]` | Single item (positional) or bulk tree (stdin JSON) |
| `hive hc list [epic-id]` | List items, optional epic/status/session filters |
| `hive hc show <id>` | Item + comments as JSON lines |
| `hive hc update <id>` | Update status (`--status`), assign (`--assign`/`--unassign`) |
| `hive hc next <epic-id>` | Next actionable leaf task; `--assign` to claim |
| `hive hc comment <id> <msg>` | Add a comment |
| `hive hc comment <id> "CHECKPOINT: msg"` | Handoff checkpoint |
| `hive hc context <epic-id>` | Epic context block; `--json` for JSON output |
| `hive hc prune` | Remove old completed items |

See `.claude/skills/hc/SKILL.md` for full agent usage guide.

## Issue Tracking

This project uses **bd** (beads) for issue tracking. Run `bd onboard` to get started.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --status in_progress  # Claim work
bd close <id>         # Complete work
bd sync               # Sync with git
```

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**

- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
