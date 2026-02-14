# Keybindings

Keybindings map keys to user commands. All keybindings must reference a command via the `cmd` field.

## Keybinding Options

| Field     | Type   | Description                                   |
| --------- | ------ | --------------------------------------------- |
| `cmd`     | string | Command name to execute (required)            |
| `help`    | string | Override help text from the command           |
| `confirm` | string | Override confirmation prompt from the command |

## Example

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
| `Recycle`        | Recycle selected session              |
| `Delete`         | Delete selected session               |
| `NewSession`     | Create a new session                  |
| `SendBatch`      | Send message to multiple agents       |
