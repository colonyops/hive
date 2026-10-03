# Agent Instructions

One repository, one Go module, four things in it:

| Path | What | Guide |
| --- | --- | --- |
| `cmd/hive/` | The **hive CLI/TUI**: a tmux-native command center that runs coding agents in isolated git clones with live status, shared context, tasks, and inter-agent messaging. The root program (`main.go` keeps `go install github.com/colonyops/hive@latest` working). | This file |
| `cmd/desktop/` | **Hive Desktop**, the Wails v3 app: an inbox that collects work into feeds, a Code area on the CLI's session engine, and agent chat workspaces. | [`cmd/desktop/AGENTS.md`](cmd/desktop/AGENTS.md) |
| `internal/` | The packages both programs share: config, sessions, git, the `hive.db` stores, messaging, hc, terminal status, HTTP plumbing. | [`docs/architecture.md`](docs/architecture.md) |
| `docs/` | hivedesktop.com: the landing page and the product docs for both programs (Zensical; pages under `docs/docs/`). The contributor docs sit beside it: `docs/architecture.md`, `docs/decisions/`, `docs/distribution.md`. | [`docs/AGENTS.md`](docs/AGENTS.md) |
| `cmd/tools/` | Development and release binaries that never ship: `adr` (decision records) and `release` (the one release run for every program: the changelog commands, the desktop publisher, and the tag and workflow dispatch that ship the CLI). | |

## Before building a feature

**Read [`docs/architecture.md`](docs/architecture.md) first.** It is the
standing reference for how the desktop app is structured and how it should
grow: the core/adapter shape, the named patterns each part follows, the
directory layout, the extension points, and the rules every PR is reviewed
against. The document describes a **target state**; where the current code
and the document disagree, the document wins for new work.

Shared `internal/` must not import charm, Wails, or anything below `cmd/`.
`cmd/hive` must not import the desktop program, and `cmd/desktop/internal/app`
consumes the shared packages only through its seam. depguard fails the lint on
a violation (`.golangci.yml`).

## Comments

Much of the existing code is densely commented. **It is not the target; do not
match it.** Draft, then delete every comment a competent reader could derive
from the code itself. The same restraint applies to prose: an ADR states the
decision and the constraint that forced it, not every alternative considered.

## Documentation

- `docs/architecture.md` is the standing architectural reference. Keep it
  current when the shape changes; it is reviewed as a spec, not as prose.
- Record notable architecture and infrastructure decisions as ADRs in
  `docs/decisions/`. Start one with
  `mise run adr:new -- "The decision, as a sentence"`. Never hand-name the
  file and never add a number (ADR adr-ids-are-not-allocated). Cite an ADR by
  its slug alone, `(ADR terminal-transport)`, and link it as
  `[…](decisions/2026-07-28-terminal-transport.md)`; `mise run check:adr`
  fails on a citation that does not resolve.
- Distribution facts for the desktop (bucket, domains, manifest schema,
  runbooks) live in `docs/distribution.md`.
- User-facing product docs are the site under `docs/docs/`; the `docs-page`
  and `docs-audit` skills govern them.

## Agent skills

The repository's own agent skills live in `.agents/skills/`
(`.claude/skills` is a symlink to it). A skill for one program carries that
program's prefix: `cli-`, `desktop-`, or `docs-`. A skill for code both
programs share (`go-enum`, `sqlc`) has no prefix. The skills Hive Desktop
ships into agent workspaces are not these; see `desktop-shipped-skills`.

## Quality gates

Every task is a mise task (`mise tasks`). The bare names are repo-wide:
code generation and the gates, which cover every program in the module
(`generate`, `test`, `lint`, `check`, `ci`). Each program's own tasks carry
its prefix: `cli:*` (`cmd/hive/tasks.toml`), `desktop:*`
(`cmd/desktop/tasks.toml`), and `docs:*` (`docs/tasks.toml`).

| | hive CLI/TUI | Hive Desktop | Site |
| --- | --- | --- | --- |
| Run | `mise run cli:dev` | `mise run desktop:dev` | `mise run docs:serve` |
| Build | `mise run cli:build` | `mise run desktop:build` | `mise run docs:build` |
| Test | `mise run test`, `mise run cli:integration` | `mise run desktop:test`, `mise run desktop:e2e` | `mise run docs:build` |

To verify a change, run `mise run check` (the Go gate, seconds) and then
`mise run ci` (every gate, minutes; needs Docker, Node, Python, and the
network).

lefthook runs the relevant gates as git hooks; `mise install` wires them up
(`postinstall` → `scripts/hooks/install.sh`).

- **pre-commit** (~0.1s): formats staged Go files and re-stages them; when a
  generator input is staged, regenerates and blocks if the committed output
  differs.
- **pre-push**: `mise run check` (generated code, ADRs, desktop migrations,
  tidy, lint, Go tests, goreleaser config), plus the frontend unit tests when
  the push touches `cmd/desktop/frontend/`. `check` needs no Python, Node, or
  Docker.
- **CI-only**: `check:deadcode`, `check:vuln`, `lint:workflows`,
  `desktop:check:bindings`, the frontend build and tests, the site build, and
  the CLI's Docker integration tests.

**`mise run ci` runs every gate, including the desktop e2e suite that GitHub
CI does not run.** Prefer it over pushing to find out.

Wails TS bindings are deliberately not hooked: they need a full app build. Run
`mise run desktop:bindings` when a service surface changes; CI checks them.

**Never bypass a hook**: no `LEFTHOOK=0`, `git commit -n`, or
`git push --no-verify`. A failing gate is a task to finish.

## The hive CLI

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
main.go             # Wrapper that keeps `go install github.com/colonyops/hive@latest` working
cmd/hive/
├── main.go         # The hive program
├── cli/            # Entry code that both main.go files run
└── internal/       # CLI-only code
    ├── app/        # Composition root: engine services plus CLI pieces
    ├── config/     # CLI config sections (views, keybindings, user commands, TUI, plugins, sources)
    ├── action/     # Keybinding action types
    ├── commands/   # CLI command handlers (urfave/cli/v3)
    ├── plugins/    # Command packs and status providers
    ├── sources/    # CLI-backed issue and PR sources (gh, tea)
    ├── styles/     # lipgloss styles
    ├── theme/      # Color palettes
    └── tui/        # Bubble Tea TUI (tree view, modals, keybindings)
cmd/desktop/        # Hive Desktop (see cmd/desktop/AGENTS.md)
cmd/tools/          # adr, release
docs/               # hivedesktop.com (docs/docs/) and the contributor docs (see docs/AGENTS.md)
internal/           # Shared with Hive Desktop
├── config/         # Engine config: loading, validation, defaults, the YAML writer, migrate/
├── core/           # doctor, eventbus
├── domain/         # Pure models: session, hc, messaging, terminal status and assess rules
├── platform/       # Drivers: git, tmux (exec, status, control, bin), proc, sqlite,
│                   # execenv, credentials, secrets, observe, workspace, tmuxtest
├── data/           # hive.db: migrations, sqlc queries, stores
├── hive/           # Service layer - orchestrates all operations
└── web/            # HTTP plumbing shared with cmd/desktop/devserver
```

UI code goes below `cmd/hive/internal/`.

### Key Files

| File                                        | Purpose                                             |
| ------------------------------------------- | --------------------------------------------------- |
| `cmd/hive/cli/cli.go`                       | CLI entry point, global flags, command registration |
| `internal/hive/service.go`                  | Service layer - coordinates sessions, git, rules    |
| `internal/config/config.go`                 | Engine config structs, loading, defaults            |
| `internal/config/validate.go`               | Template data structs, validation                   |
| `cmd/hive/internal/config/config.go`        | CLI config: views, keybindings, user commands       |
| `cmd/hive/internal/tui/model.go`            | TUI model, update loop, view rendering              |
| `cmd/hive/internal/tui/views/sessions/tree_view.go` | Session tree with status indicators         |
| `internal/domain/terminal/assess/rules_*.go` | AI agent status detection rules                     |

### Development

#### Commands

```bash
mise run cli:dev              # Run the TUI with the dev config (supports CLI args)
mise run cli:dev -- new       # Example: run 'hive new' with dev config
mise run cli:start            # Run the TUI with your global config (supports CLI args)
mise run cli:build            # Build with goreleaser
mise run cli:container        # Build and launch an ephemeral Docker container with hive pre-installed
mise run cli:integration      # Docker-based integration tests
mise run test                 # Run every Go test in the module (CLI, shared, desktop)
mise run lint                 # Run golangci-lint
mise run check                # The Go gate; read-only
mise run ci                   # Every gate, including the site, the frontend, and e2e
mise run tidy                 # go mod tidy (the counterpart of check:tidy that changes files)
mise run coverage             # Generate coverage report
```

#### Manual Testing

Use `mise run cli:container` to manually test hive end-to-end. It builds the current branch and drops you into an isolated Docker container with hive installed and tmux available — no need to install a local binary or worry about polluting your dev environment.

```bash
mise run cli:container
# Inside the container (hive is aliased to 'hv'):
hv new --remote <url> "my-session"
hv ls
```

This is the preferred way to test CLI/TUI behavior, session creation, branch templates, tmux integration, and anything that requires a real git environment. Do NOT attempt to test by manually building and replacing a binary in your PATH.

#### Environment

Dev environment uses `cmd/hive/dev/config.dev.yaml` and `.data/` for isolation:

```bash
HIVE_LOG_LEVEL=debug
HIVE_LOG_FILE=./dev.log
HIVE_CONFIG=./cmd/hive/dev/config.dev.yaml
HIVE_DATA_DIR=./.data
```

### Code Generation

#### go-enum

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

#### sqlc

Queries live in `internal/data/db/queries/`. Generated files (`queries*.sql.go`, `models.go`) are committed and must never be edited manually.

```bash
mise run generate    # regenerates after SQL or sqlc.yaml changes
sqlc generate        # directly
```

**Adding a query:** write the annotated SQL (`:one`, `:many`, `:exec`), run generation, then call via `s.db.Queries().FunctionName(ctx, ...)`.

**Type overrides:** when a column stores a domain enum, add an override to `sqlc.yaml` so the generated code uses the Go type directly instead of `string`. The go-enum type satisfies the required interfaces via `MarshalText`/`UnmarshalText`.

Always commit the generated `*.sql.go` and `models.go` alongside the SQL changes in the same commit.

### Code Patterns

#### Integration Tests

Integration tests live in `test/integration/` and require a compiled binary. They use the `integration` build tag and are excluded from the standard `mise run test` run.

**CRITICAL: NEVER run integration tests directly on the host** (e.g. `go test -tags integration ./test/integration/...` or `mise run test -- -tags integration` run outside a container). These tests spawn real tmux sessions and subprocesses that have crashed host tmux/dev environments. Integration tests exercise tmux session lifecycle and are only safe inside the isolated Docker environment.

Always run integration tests via the Docker-based task instead:

```bash
mise run cli:integration    # builds the project and runs integration tests inside Docker
```

Use `mise run cli:container` for interactive manual testing in the same isolated environment (see "Manual Testing" above).

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

#### Bubble Tea (TUI)

Standard Model/Update/View pattern. Key messages:

- `sessionsLoadedMsg` - Sessions fetched from store
- `sessions.GitStatusBatchCompleteMsg` - Git status for all sessions
- `sessions.TerminalPollTickMsg` - Terminal status polling tick
- `actionCompleteMsg` - Keybinding action finished

#### Configuration

Two validation phases:

1. **Basic** (`Validate()`) - Struct validation, required fields
2. **Deep** (`ValidateDeep()`) - File access, template syntax, regex patterns

#### Templates

Commands support Go templates with `shq` function for shell quoting:

```yaml
spawn:
  - my-script {{ .Name | shq }} {{ .Path | shq }}
```

Available variables vary by context - see `internal/config/validate.go` for `*TemplateData` structs.

#### Error Handling

Never silently discard errors. If an error cannot be presented to the user (e.g., in background polling, cache refresh, or TUI status fetching), log it at an appropriate level (`debug` for expected/transient failures, `warn` for configuration problems). Prefer degraded behavior with logging over silent fallbacks — for example, show a `StatusMissing` indicator instead of dropping an item from the UI.

#### Keybinding Precedence

The TUI dispatches keystrokes through three layers, in this order:

1. **Layer 1: hardcoded, non-overridable** - `ctrl+c`, `esc`, `tab`, and `shift+tab` in normal-mode handling, plus modal-lifecycle dismissal keys (`esc` / `q`) inside dialogs and modals.
2. **Layer 2: configurable via `KeybindingResolver`** - every other user-overridable key. Default bindings live in `defaultViewsConfig` (`cmd/hive/internal/config/config_views.go`), and user config in `cfg.Views.{Global,Sessions,Tasks,Review}.Keybindings` overrides those defaults because `maps.Copy(merged, user)` overlays user values onto the merged map.
3. **Layer 3: bubbles list internals** - the underlying list component claims keys like `g`, `G`, `j`, `k`, `h`, `l`, `u`, `d`, `f`, `b`, `/`, `?`, `q`, and `esc`. This is intentionally out of scope for hive keybinding configuration: the resolver consumes configured bindings before the list sees them.

When adding a new overridable key, do not add a new `if keyStr == "X"` block in view code. Register an `action.Type` in `cmd/hive/internal/action/type.go`, add a default `UserCommand` in `defaultUserCommands` (`cmd/hive/internal/config/config.go`), bind it in `defaultViewsConfig`, and dispatch it from `cmd/hive/internal/tui/model_handlers.go`.

#### Session States

```
(new) ──► active ──► recycled ──► (deleted)
              │           │
              └──► corrupted ──► (deleted)
```

## Honeycomb (hc) — Multi-Agent Task Coordination

`hive hc` is the built-in task coordination system for multi-agent workflows. A conductor creates epics and tasks; workers claim and complete them.

#### Quick Reference

```bash
# Conductor: create work (simple)
echo '{"title":"Epic","type":"epic","children":[{"title":"Task","type":"task"}]}' | hive hc create

# Conductor: create work with blocker dependencies (ref/blockers are local labels, not stored)
echo '{
  "title": "Auth System",
  "type": "epic",
  "children": [
    {"ref": "jwt", "title": "JWT middleware", "type": "task"},
    {"title": "Login endpoint", "type": "task", "blockers": ["jwt"]}
  ]
}' | hive hc create

# Worker: claim next task
hive hc next <epic-id> --assign

# Worker: record progress
hive hc comment <id> "implemented X"
hive hc comment <id> "CHECKPOINT: stopping here, Y still needed"

# Worker: complete task
hive hc update <id> --status done

# Worker: manage blockers after creation
hive hc update <id> --add-blocker <blocker-id>    # mark task as blocked by another
hive hc update <id> --remove-blocker <blocker-id>  # remove a blocker

# Get context for an epic (markdown for AI consumption)
hive hc context <epic-id>

# List tasks
hive hc list                          # open items (default)
hive hc list --all                    # all items regardless of status
hive hc list <epic-id>                # open items under an epic
hive hc list --status done            # filter by specific status
hive hc list --session <session-id>   # filter by session
```

#### Key Commands

| Command | Purpose |
| ------- | ------- |
| `hive hc create [title]` | Single item (positional) or bulk tree (stdin JSON) |
| `hive hc list [epic-id]` | List open items (use `--all` for everything) |
| `hive hc show <id>` | Item + comments as JSON lines |
| `hive hc update <id>` | Update status (`--status`), assign (`--assign`/`--unassign`), manage blockers (`--add-blocker`/`--remove-blocker`) |
| `hive hc next <epic-id>` | Next actionable leaf task; `--assign` to claim |
| `hive hc comment <id> <msg>` | Add a comment |
| `hive hc comment <id> "CHECKPOINT: msg"` | Handoff checkpoint |
| `hive hc context <epic-id>` | Epic context block; `--json` for JSON output |
| `hive hc prune` | Remove old completed items |

See `claude-plugin/hive/skills/hc/SKILL.md` for the full agent usage guide.

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
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
