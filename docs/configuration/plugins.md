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
    entry: ~/.config/hive/plugins/init.lua
```

!!! info "Auto-detection"
    Most plugins auto-detect their dependencies at startup. You only need to set `enabled: true` — if the required CLI tool isn't installed, the plugin silently deactivates. No errors, no configuration needed.

## Lua Plugin Entry

Hive reserves `plugins.lua` for a user-provided Lua plugin entrypoint.

```yaml
plugins:
  lua:
    entry: ~/.config/hive/plugins/init.lua
```

If `plugins.lua.entry` is unset, Hive looks for `~/.config/hive/plugins/init.lua` by convention. A missing default file is a no-op. Set `plugins.lua.entry` only when you want Hive to load a different entry file; in that case the file must exist.

The Lua module root is derived from the directory containing the entry file. Organize helper modules beside that entry file and load them with normal Lua `require()` behavior.

### Entrypoint Contract

Hive executes the entry file at startup and expects it to return exactly one function:

```lua
return function(hive)
  -- register commands here
end
```

If the file does not compile, returns the wrong value, or raises an error while initializing, Hive logs a plugin initialization warning and continues startup without loading the Lua commands.

### V1 Host API

Lua plugins get a deliberately small `hive` table:

- `hive.commands(map)` registers one or more user commands.
- `hive.log.debug(msg)`, `hive.log.info(msg)`, `hive.log.warn(msg)`, `hive.log.error(msg)` write to the normal hive log.
- `hive.plugin.name`, `hive.plugin.entry`, and `hive.plugin.module_root` expose plugin metadata.

`hive.commands(map)` accepts the same command names used elsewhere in hive config. Each value must be a Lua table describing one shell-backed command.

```lua
return function(hive)
  hive.commands({
    LuaHello = {
      sh = "printf 'hello from lua\\n'",
      help = "example Lua-backed command",
      scope = { "sessions" },
      confirm = "Run LuaHello?",
      silent = true,
      exit = "$HIVE_POPUP",
    },
  })
end
```

Supported command fields in v1:

| Field     | Type                  | Meaning |
| --------- | --------------------- | ------- |
| `sh`      | `string`              | Shell command template to execute. This is required. |
| `help`    | `string`              | Help text shown in the command palette. |
| `scope`   | `string[]`            | Views where the command is available. Valid values match normal user commands: `global`, `sessions`, `messages`, `review`, `todos`, `tasks`. Omit it for global availability. |
| `confirm` | `string`              | Confirmation prompt shown before execution. |
| `silent`  | `boolean`             | Skip the loading popup for fast commands. |
| `exit`    | `boolean` or `string` | Exit condition, using the same rules as normal user commands such as `true` or `$HIVE_POPUP`. |

Lua plugin commands intentionally do not support the broader user-command surface in v1. In particular, `action`, `windows`, `options`, and `form` are rejected.

### Module Layout And `require()`

The Lua module root is the directory containing the entry file. Hive configures `require()` relative to that directory, so helper modules can live alongside `init.lua` and use normal dot-notation imports.

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

Use standard Lua `require()` naming rooted at the entry file directory. For example, `commands/hello.lua` becomes `require("commands.hello")`, and `commands/hello/init.lua` becomes `require("commands.hello")`.

### V1 Boundaries

The Lua plugin API is intentionally narrow in v1:

- No status hooks or custom status providers.
- No event bus or session lifecycle APIs.
- No package-management helpers, dependency installer, or remote module loading.
- No callback-style command fields; command definitions are declarative tables only.

The supported v1 use case is: load local Lua modules from the plugin directory, then register shell-backed commands through `hive.commands(map)`.

### End-To-End Smoke Test

The preferred way to try a Lua plugin end-to-end is inside the project container so you do not have to install a development binary locally:

```bash
mise container
```

Inside the container:

```bash
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

Open the command palette with `:`, run `LuaHello`, then verify that `.lua-plugin-output` was created in the selected session directory. If you want to keep the plugin somewhere else, set `plugins.lua.entry` to that alternate `init.lua` path and keep using standard Lua `require()` from that entry file's directory.

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
    enabled: true        # auto-detected (requires `claude` CLI)
    cache_ttl: 30s       # status cache duration
    yellow_threshold: 60 # yellow warning above this % (default: 60)
    red_threshold: 80    # red warning above this % (default: 80)
    model_limit: 200000  # context token limit (default: 200000)
```

### Commands Provided

| Command      | Description                      | Default Key |
| ------------ | -------------------------------- | ----------- |
| `ClaudeFork` | Fork Claude session in new window | —          |

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
    enabled: true      # auto-detected (requires `gh` CLI)
    results_cache: 8m  # how often to refresh PR status (default: 8m)
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

| Label    | Color   | Meaning          |
| -------- | ------- | ---------------- |
| `PR open`   | Green   | PR is open       |
| `PR draft`  | Muted   | PR is a draft    |
| `PR merged` | Primary | PR was merged    |
| `PR closed` | Muted   | PR was closed    |

## Beads Plugin

The Beads plugin provides issue tracking integration with the `bd` (beads) CLI. Auto-detected when `bd` CLI is installed.

```yaml
plugins:
  beads:
    enabled: true        # auto-detected (requires `bd` CLI)
    results_cache: 30s   # how often to refresh issue counts (default: 30s)
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

| Display  | Color   | Meaning                    |
| -------- | ------- | -------------------------- |
| `BD 3/5` | Primary | 3 of 5 issues closed       |
| `BD 5/5` | Green   | All issues closed          |

## LazyGit Plugin

The lazygit plugin provides commands to open lazygit in a tmux popup. Auto-detected when `lazygit` is installed.

```yaml
plugins:
  lazygit:
    enabled: true  # auto-detected (requires `lazygit`)
```

### Commands Provided

| Command          | Description                 | Default Key |
| ---------------- | --------------------------- | ----------- |
| `LazyGitOpen`    | Open lazygit (full popup)   | —           |
| `LazyGitCommits` | Open lazygit commit log     | —           |

## Neovim Plugin

The neovim plugin provides a command to open neovim in the session's tmux session. Auto-detected when `nvim` is installed.

```yaml
plugins:
  neovim:
    enabled: true  # auto-detected (requires `nvim`)
```

### Commands Provided

| Command      | Description                          | Default Key |
| ------------ | ------------------------------------ | ----------- |
| `NeovimOpen` | Open neovim in new tmux window       | —           |

## Context Directory Plugin

The context directory plugin provides commands to open context directories in the system file browser. Always available on macOS and Linux.

```yaml
plugins:
  contextdir:
    enabled: true  # always available on macOS/Linux
```

### Commands Provided

| Command              | Description                      | Default Key |
| -------------------- | -------------------------------- | ----------- |
| `ContextOpenSession` | Open session's `.hive` directory | —           |
| `ContextOpenAll`     | Open all context directories     | —           |
