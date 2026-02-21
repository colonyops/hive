---
icon: lucide/keyboard
---

# Keybindings

Keybindings map keys to user commands. All keybindings reference a command via the `cmd` field.

```yaml
keybindings:
  r:
    cmd: Recycle # System default
  d:
    cmd: Delete # System default
  o:
    cmd: open # User-defined command
  t:
    cmd: tidy
    confirm: "Run tidy on this session?" # Override command's confirm
```

!!! info
    Keybindings reference commands by name. Both system default commands (like `Recycle`) and user-defined commands can be bound. You can override the command's `help` and `confirm` fields per-binding.

## Keybinding Options

| Field     | Type   | Description                                   |
| --------- | ------ | --------------------------------------------- |
| `cmd`     | string | Command name to execute (required)            |
| `help`    | string | Override help text from the command           |
| `confirm` | string | Override confirmation prompt from the command |

## Default Keybindings

| Key        | Command              | Description                          |
| ---------- | -------------------- | ------------------------------------ |
| `:`        | —                    | Open command palette                 |
| `v`        | —                    | Toggle preview sidebar               |
| `r`        | Recycle              | Recycle session                      |
| `d`        | Delete               | Delete session (or tmux window)      |
| `n`        | NewSession           | New session (when repos discovered)  |
| `enter`    | TmuxOpen             | Open/attach tmux session             |
| `ctrl+d`   | TmuxKill             | Kill tmux session                    |
| `A`        | AgentSend            | Send Enter to agent                  |
| `R`        | RenameSession        | Rename session                       |
| `G`        | GroupSet             | Set session group                    |
| `J`        | NextActive           | Jump to next active session          |
| `K`        | PrevActive           | Jump to previous active session      |
| `p`        | TmuxPopUp            | Popup tmux session                   |
| `g`        | —                    | Refresh git statuses                 |
| `tab`      | —                    | Switch views                         |
| `q`        | —                    | Quit                                 |

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
