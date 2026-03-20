---
icon: lucide/search
---

# Session Picker (Experimental)

!!! warning "Experimental"
    Session picker behavior, flags, and output formats are experimental and may change in future releases.

`hive x pick` opens a fuzzy session picker directly in the terminal. It shows all active sessions with live status indicators, lets you filter by name or repository, and switches your tmux client to the selected session.

## Usage

```bash
hive x pick                  # open picker, switch to selection
hive x pick --print          # print selected session ID instead of switching
hive x pick --repo myrepo    # pre-filter by repository
hive x pick --hide-current   # exclude the session you're currently in
```

## Keybindings

| Key | Action |
| --- | --- |
| `↑` / `ctrl+k` | Move up |
| `↓` / `ctrl+j` | Move down |
| `tab` | Cycle status filter (all → active → approval → ready → missing) |
| `enter` | Switch to selected session |
| `esc` / `ctrl+c` | Cancel |

## Flags

| Flag | Description |
| ---- | ----------- |
| `--status`, `-s` | Pre-set the status filter (`active`, `approval`, `ready`, `missing`) |
| `--repo`, `-r` | Filter by repository remote URL (substring match) |
| `--print`, `-p` | Print selected session info instead of switching tmux |
| `--format`, `-f` | Output format for `--print`: `id` (default), `name`, `path`, `json` |
| `--hide-current` | Hide the session you are currently in |
| `--no-recents` | Don't prioritize recently-used sessions |

## Quick Navigation Popup via Tmux

Bind `hive x pick` to a tmux key to get an instant session switcher from anywhere:

```bash
# ~/.tmux.conf
bind-key f run-shell 'tmux new-window "hive x pick"'
```

Press `prefix + f` from any window. The picker opens in a new window, switches your client to the chosen session, and the picker window closes automatically. Press `esc` to cancel without switching.
