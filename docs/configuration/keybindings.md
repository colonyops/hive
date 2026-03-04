---
icon: lucide/keyboard
---

# Keybindings

Keybindings map keys to user commands. Keybindings are configured per-view under the `views` section, with a `global` section for bindings available in all views.

```yaml
views:
  global:
    keybindings:
      "?":
        cmd: HiveInfo
  sessions:
    keybindings:
      o:
        cmd: open # User-defined command
      t:
        cmd: tidy
        confirm: "Run tidy on this session?"
  tasks:
    keybindings:
      r:
        cmd: TasksRefresh
```

!!! info
    Keybindings reference commands by name. Both system default commands (like `Recycle`) and user-defined commands can be bound. You can override the command's `help` and `confirm` fields per-binding.

!!! warning "Deprecated: top-level keybindings"
    The top-level `keybindings` field is deprecated. Move entries to `views.sessions.keybindings` instead. Existing top-level keybindings are automatically migrated to the sessions view.

## Keybinding Options

| Field     | Type   | Description                                   |
| --------- | ------ | --------------------------------------------- |
| `cmd`     | string | Command name to execute (required)            |
| `help`    | string | Override help text from the command           |
| `confirm` | string | Override confirmation prompt from the command |

## Default Keybindings

### Sessions View

| Key        | Command              | Description                          |
| ---------- | -------------------- | ------------------------------------ |
| `r`        | Recycle              | Recycle session                      |
| `d`        | Delete               | Delete session (or tmux window)      |
| `n`        | NewSession           | New session (when repos discovered)  |
| `enter`    | TmuxOpen             | Open/attach tmux session             |
| `ctrl+d`   | TmuxKill             | Kill tmux session                    |
| `A`        | AgentSend            | Send Enter to agent                  |
| `R`        | RenameSession        | Rename session                       |
| `ctrl+g`   | GroupSet             | Set session group                    |
| `J`        | NextActive           | Jump to next active session          |
| `K`        | PrevActive           | Jump to previous active session      |
| `t`        | TodoPanel            | Open todo panel                      |
| `p`        | TmuxPopUp            | Popup tmux session                   |

### Tasks View

| Key  | Command              | Description                     |
| ---- | -------------------- | ------------------------------- |
| `r`  | TasksRefresh         | Refresh task list               |
| `f`  | TasksFilter          | Cycle status filter             |
| `y`  | TasksCopyID          | Copy task ID to clipboard       |
| `v`  | TasksTogglePreview   | Toggle preview panel            |
| `s`  | TasksSelectRepo      | Select repository scope         |

### Hard-coded Keys (all views)

| Key        | Description                          |
| ---------- | ------------------------------------ |
| `:`        | Open command palette                 |
| `tab`      | Switch views                         |
| `q`        | Quit                                 |

### Sessions View Navigation

| Key  | Description              |
| ---- | ------------------------ |
| `v`  | Toggle preview sidebar   |
| `g`  | Refresh git statuses     |

### Tasks View Navigation

| Key          | Description              |
| ------------ | ------------------------ |
| `space`      | Expand/collapse item     |
| `enter`, `l` | Open detail pane         |
| `h`, `esc`   | Back to tree             |

For todo panel interaction keys (`enter`, `c`, `d`, `tab`, etc.), see [Todos (Experimental)](../getting-started/todos.md).

## Per-View Keybinding Resolution

When a key is pressed, hive resolves bindings in this order:

1. **View-specific bindings** — checked first (e.g., `views.sessions.keybindings`)
2. **Global bindings** — checked as fallback (e.g., `views.global.keybindings`)

A view-specific binding overrides a global binding for the same key.

## Built-in Palette Commands

These commands are available in the command palette (`:`) but have no default keybinding:

| Command          | Description                           |
| ---------------- | ------------------------------------- |
| `FilterAll`      | Show all sessions                     |
| `FilterActive`   | Show sessions with active agents      |
| `FilterApproval` | Show sessions needing approval        |
| `FilterReady`    | Show sessions with idle agents        |
| `GroupToggle`    | Toggle between repo/group tree view   |
| `SendBatch`      | Send message to multiple agents       |
| `TmuxStart`      | Start tmux session in background      |
