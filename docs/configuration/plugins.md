---
icon: lucide/puzzle
---

# Plugins

Plugins extend hive with custom commands and status providers. Plugins auto-detect their dependencies at startup — if the required CLI tool is installed, the plugin activates automatically.

```yaml
plugins:
  tmux:
    enabled: true
  claude:
    enabled: true
    yellow_threshold: 60
    red_threshold: 80
  github:
    enabled: true
    results_cache: 8m
  beads:
    enabled: true
    results_cache: 30s
  lua:
    enabled: true
    entry: ~/.config/hive/plugins/init.lua
```

!!! info "Auto-detection"
    Most plugins auto-detect their dependencies at startup. You only need to set `enabled: true` — if the required CLI tool isn't installed, the plugin silently deactivates. No errors, no configuration needed.

## Lua Plugin Entry

Hive can load user commands from a local Lua entry file.

```yaml
plugins:
  lua:
    enabled: true # nil = auto-detect, false to disable
    entry: ~/.config/hive/plugins/init.lua # omit to use the default discovery path
```

When `entry` is omitted, Hive checks `~/.config/hive/plugins/init.lua` automatically; a missing file there is silently ignored. When `entry` is set explicitly, the file must exist. Set `enabled: false` to keep the entry file on disk but skip loading it.

### Scaffolding

`hive plugin init` writes a starter `init.lua` and `commands/hello.lua` into the default plugin directory, where the loader auto-discovers it on next run:

```bash
hive plugin init
# Scaffolded Lua plugin at ~/.config/hive/plugins
```

| Flag             | Effect                                                                                   |
| ---------------- | ---------------------------------------------------------------------------------------- |
| `--path <dir>`   | Write to `<dir>` instead of the default. Set `plugins.lua.entry` to `<dir>/init.lua`.    |
| `--force`        | Overwrite an existing `init.lua` or `commands/hello.lua`. Other files are preserved.     |

Without `--force`, the command refuses if either generated file already exists.

The entry file should return a function that registers one or more commands:

```lua
return function(hive)
  hive.commands({
    LuaHello = {
      sh = "printf 'hello from lua\\n'",
      help = "example Lua-backed command",
      scope = { "sessions" },
    },
  })
end
```

Hive loads helper modules relative to that entry file, so `require("commands.hello")` works for files placed alongside `init.lua`.

Supported command fields:

| Field     | Type                  | Meaning                                                                |
| --------- | --------------------- | ---------------------------------------------------------------------- |
| `sh`      | `string`              | Shell command template to execute. This is required.                   |
| `help`    | `string`              | Help text shown in the command palette.                                |
| `scope`   | `string[]`            | Views where the command is available. Omit it for global availability. |
| `confirm` | `string`              | Confirmation prompt shown before execution.                            |
| `silent`  | `boolean`             | Skip the loading popup for fast commands.                              |
| `exit`    | `boolean` or `string` | Exit condition, using the same rules as normal user commands.          |

Lua-backed commands are intentionally limited to shell-backed command registration. Fields such as `action`, `windows`, `options`, and `form` are not supported here.

Example layout:

```text
~/.config/hive/plugins/
├── init.lua
└── commands/
    └── hello.lua
```

`init.lua`:

```lua
local commands = require("commands.hello")

return function(hive)
  hive.commands(commands)
end
```

`commands/hello.lua`:

```lua
return {
  LuaHello = {
    sh = "printf 'lua command ran' > .lua-plugin-output",
    help = "lua command",
    scope = { "sessions" },
    silent = true,
  },
}
```

To try it:

```bash
mise container
mkdir -p ~/.config/hive/plugins/commands
cat > ~/.config/hive/plugins/init.lua <<'EOF'
local commands = require("commands.hello")

return function(hive)
  hive.commands(commands)
end
EOF

cat > ~/.config/hive/plugins/commands/hello.lua <<'EOF'
return {
  LuaHello = {
    sh = "printf 'lua command ran' > .lua-plugin-output",
    help = "lua command",
    scope = { "sessions" },
    silent = true,
  },
}
EOF

hv new --remote <url> lua-smoke
hv
```

Open the command palette with `:`, run `LuaHello`, and confirm that `.lua-plugin-output` was created in the selected session directory.

### hive.ticker

Schedule callbacks to run repeatedly or after a delay.

| Function                          | Purpose                                           |
| --------------------------------- | ------------------------------------------------- |
| `hive.ticker.every(duration, fn)` | Run `fn` every `duration`. Returns a handle.      |
| `hive.ticker.after(duration, fn)` | Run `fn` once after `duration`. Returns a handle. |
| `handle:cancel()`                 | Cancel the ticker. Idempotent.                    |

`duration` accepts any Go duration string (e.g. `"5s"`, `"1m30s"`).

```lua
return function(hive)
  local heartbeat = hive.ticker.every("5s", function()
    hive.log.info("still alive")
  end)

  hive.ticker.after("60s", function()
    hive.log.info("stopping heartbeat")
    heartbeat:cancel()
  end)
end
```

!!! warning "1-second minimum interval"
    Anything shorter raises a Lua error rather than silently clamping. Sub-second polling is not supported — pick reasonable cadences.

!!! note "Callbacks run serially"
    All ticker fires share the same dispatcher goroutine as your entrypoint and command handlers, so a long-running callback delays every other ticker on the plugin's Lua state.

!!! warning "Backpressure: drop + log"
    If a callback runs longer than the tick interval, additional ticks queue in a bounded buffer (currently 64 items per plugin). When the buffer is full, further ticks are dropped and a warning is logged. The module does **not** coalesce or skip cleanly — it drops, so plan for callbacks that may not fire on every tick under heavy load.

!!! note "Shutdown cancels everything"
    When hive shuts down or the plugin is reloaded, every outstanding ticker is cancelled and its callback is released for GC.

### hive.json

Encode and decode JSON values for IPC, config files, or HTTP integrations.

| Function                         | Purpose                                               |
| -------------------------------- | ----------------------------------------------------- |
| `hive.json.encode(value, opts?)` | Encode a Lua value to a JSON string.                  |
| `hive.json.decode(string)`       | Decode a JSON string to a Lua value.                  |
| `hive.json.array(table)`         | Tag a Lua table so it always encodes as a JSON array. |

`opts` is an optional table. Set `pretty = true` to pretty-print the output with two-space indentation.

```lua
return function(hive)
  local payload = hive.json.encode({
    sessions = { "alpha", "beta" },
    count    = 2,
  }, { pretty = true })
  hive.log.info(payload)

  local decoded = hive.json.decode('{"foo":[1,2,3]}')
  hive.log.info("first item: " .. tostring(decoded.foo[1]))
end
```

#### Array vs object detection

A Lua table encodes as a JSON array when:

1. It was passed through `hive.json.array(...)`, **or**
2. Every key is a positive integer `1..n` with no holes.

Otherwise it encodes as a JSON object. Empty unmarked tables encode as `{}` because Lua cannot distinguish "empty array" from "empty object" without a hint:

```lua
hive.json.encode({})                      -- "{}"
hive.json.encode(hive.json.array({}))     -- "[]"
```

Decoding `[]` from JSON produces a marked table, so `decode` followed by `encode` round-trips empty arrays as `[]` rather than `{}`.

!!! warning "Number precision"
    All Lua numbers are 64-bit floats, so integer values larger than 2^53 (≈ 9 × 10^15) lose precision on the round-trip. If you need full-width integer fidelity, encode such values as strings.

!!! note "JSON null becomes Lua nil"
    JSON `null` decodes to `nil`, which Lua treats as "field absent". A re-encode of a decoded object will therefore omit any field that was originally `null`. There is no `hive.json.null` sentinel in this release.

!!! warning "Cycles raise an error"
    `encode` rejects self-referencing tables with a Lua error. Detect cycles before encoding if your data may contain back-references.

### hive.kv

Persist string values across hive restarts. Backed by the same SQLite store that other Hive plugins use, but every key is automatically prefixed with `lua:` so a Lua plugin cannot read or stomp keys owned by other components.

| Function | Purpose |
| -------- | ------- |
| `hive.kv.set(key, value)` | Store `value` under `key`. Both must be strings. |
| `hive.kv.get(key)` | Return the value for `key`, or `nil` if missing. |
| `hive.kv.delete(key)` | Remove `key`. No-op if absent. |

```lua
return function(hive)
  local last = hive.kv.get("last_run")
  if last then
    hive.log.info("previous run: " .. last)
  end
  hive.kv.set("last_run", os.date("!%Y-%m-%dT%H:%M:%SZ"))
end
```

!!! note "Strings only in v1"
    Both arguments to `set` go through `CheckString` — Lua numbers are coerced via `tostring`, but tables, booleans, and `nil` are rejected with a Lua error. Use `hive.json.encode` if you need to persist a structured value.

!!! note "Missing keys return `nil`"
    `get` is the only op that distinguishes missing from present; `set` and `delete` raise a Lua error on store failure but otherwise succeed silently. `delete` on a missing key is a no-op.

!!! warning "Empty keys are rejected"
    `set("", v)`, `get("")`, and `delete("")` all raise a Lua error. Pick a non-empty key.

## Tmux Plugin

The tmux plugin provides default commands for session management using bundled scripts (`hive-tmux`, `agent-send`) that are auto-extracted to `$HIVE_DATA_DIR/bin/`.

```yaml
plugins:
  tmux:
    enabled: true # true by default, set false to disable
```

### Commands Provided

| Command          | Description                     | Default Key |
| ---------------- | ------------------------------- | ----------- |
| `TmuxOpen`       | Open/attach tmux session        | `enter`     |
| `TmuxStart`      | Start tmux session (background) | —           |
| `TmuxKill`       | Kill tmux session               | `ctrl+d`    |
| `TmuxPopUp`      | Popup tmux session              | `p`         |
| `AgentSend`      | Send Enter to agent             | `A`         |
| `AgentSendClear` | Send /clear to agent            | —           |

## Claude Plugin

The Claude plugin provides integration with Claude Code sessions — forking sessions and monitoring context usage.

```yaml
plugins:
  claude:
    enabled: true # auto-detected (requires `claude` CLI)
    cache_ttl: 30s # status cache duration
    yellow_threshold: 60 # yellow warning above this % (default: 60)
    red_threshold: 80 # red warning above this % (default: 80)
    model_limit: 200000 # context token limit (default: 200000)
```

### Commands Provided

| Command      | Description                       | Default Key |
| ------------ | --------------------------------- | ----------- |
| `ClaudeFork` | Fork Claude session in new window | —           |

### Context Analytics

Session names are colored based on context usage:

- **Default color** — Below 60% (no warning)
- **Yellow** — 60-79% (approaching limit)
- **Red** — 80%+ (at or near limit)

!!! note
    The plugin detects active session IDs by scanning `~/.claude/projects/{project-dir}/` for recently modified UUID session files (within 5 minutes). No manual metadata configuration needed.

### Usage

```yaml
# Add keybinding for fork
keybindings:
  f:
    cmd: ClaudeFork

# Or invoke via command palette
# :ClaudeFork
```

## GitHub Plugin

The GitHub plugin provides PR status display and GitHub CLI commands. Auto-detected when `gh` CLI is installed.

```yaml
plugins:
  github:
    enabled: true # auto-detected (requires `gh` CLI)
    results_cache: 8m # how often to refresh PR status (default: 8m)
```

### Commands Provided

| Command          | Description                | Default Key |
| ---------------- | -------------------------- | ----------- |
| `GithubOpenRepo` | Open repo in browser       | —           |
| `GithubOpenPR`   | View current PR in browser | —           |
| `GithubPRStatus` | Show PR status (popup)     | —           |
| `GithubPRCreate` | Create PR in browser       | —           |

### Status Display

Sessions with an associated PR show a status indicator:

| Label       | Color   | Meaning       |
| ----------- | ------- | ------------- |
| `PR open`   | Green   | PR is open    |
| `PR draft`  | Muted   | PR is a draft |
| `PR merged` | Primary | PR was merged |
| `PR closed` | Muted   | PR was closed |

## Beads Plugin

The Beads plugin provides issue tracking integration with the `bd` (beads) CLI. Auto-detected when `bd` CLI is installed.

```yaml
plugins:
  beads:
    enabled: true # auto-detected (requires `bd` CLI)
    results_cache: 30s # how often to refresh issue counts (default: 30s)
```

### Commands Provided

| Command      | Description              | Default Key |
| ------------ | ------------------------ | ----------- |
| `BeadsReady` | Show ready tasks (popup) | —           |
| `BeadsList`  | List all issues (popup)  | —           |
| `BeadsTUI`   | Open perles kanban TUI   | —           |

`BeadsTUI` is only registered if the `perles` CLI is available.

### Status Display

Sessions with a `.beads` directory show issue progress:

| Display  | Color   | Meaning              |
| -------- | ------- | -------------------- |
| `BD 3/5` | Primary | 3 of 5 issues closed |
| `BD 5/5` | Green   | All issues closed    |

## LazyGit Plugin

The lazygit plugin provides commands to open lazygit in a tmux popup. Auto-detected when `lazygit` is installed.

```yaml
plugins:
  lazygit:
    enabled: true # auto-detected (requires `lazygit`)
```

### Commands Provided

| Command          | Description               | Default Key |
| ---------------- | ------------------------- | ----------- |
| `LazyGitOpen`    | Open lazygit (full popup) | —           |
| `LazyGitCommits` | Open lazygit commit log   | —           |

## Neovim Plugin

The neovim plugin provides a command to open neovim in the session's tmux session. Auto-detected when `nvim` is installed.

```yaml
plugins:
  neovim:
    enabled: true # auto-detected (requires `nvim`)
```

### Commands Provided

| Command      | Description                    | Default Key |
| ------------ | ------------------------------ | ----------- |
| `NeovimOpen` | Open neovim in new tmux window | —           |

## Context Directory Plugin

The context directory plugin provides commands to open context directories in the system file browser. Always available on macOS and Linux.

```yaml
plugins:
  contextdir:
    enabled: true # always available on macOS/Linux
```

### Commands Provided

| Command              | Description                      | Default Key |
| -------------------- | -------------------------------- | ----------- |
| `ContextOpenSession` | Open session's `.hive` directory | —           |
| `ContextOpenAll`     | Open all context directories     | —           |
