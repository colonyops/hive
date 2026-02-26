# Tmux Setup for Hive

This covers how to configure tmux to work well with hive, including the theme,
keybindings, and session management.

## Dependencies

Install via Homebrew:

```bash
brew install bash bc fzf lazygit
brew install --cask font-jetbrains-mono-nerd-font
```

- **bash** — macOS ships with bash 3.2; tokyo-night-tmux requires 4.2+
- **bc** — required by tokyo-night-tmux for netspeed/git widgets
- **fzf** — fuzzy finder for session picker
- **lazygit** — terminal git UI, faster than gitk on large repos
- **font-jetbrains-mono-nerd-font** — required by tokyo-night-tmux for icons

Set the font in your terminal (e.g. Ghostty):

```
font-family = JetBrainsMono NF
```

## TPM Plugins

Add to `~/.config/tmux/tmux.conf`:

```
set -g @plugin 'tmux-plugins/tpm'
set -g @plugin 'tmux-plugins/tmux-resurrect'
set -g @plugin 'tmux-plugins/tmux-continuum'
set -g @plugin 'janoamaral/tokyo-night-tmux'

set -g @resurrect-capture-pane-contents 'on'
set -g @continuum-restore 'on'
set -g @continuum-save-interval '15'

set -g @tokyo-night-tmux_show_datetime 0
set -g @tokyo-night-tmux_show_path 0
set -g @tokyo-night-tmux_show_git 0
set -g @tokyo-night-tmux_show_panes 0
set -g @tokyo-night-tmux_window_id_style none
set -g @tokyo-night-tmux_pane_id_style hide
set -g @tokyo-night-tmux_zoom_id_style none

run '~/.config/tmux/plugins/tpm/tpm'

# Override selection highlight after TPM
set -g mode-style "fg=#1a1b26,bg=#ffffff"
```

Clone TPM:

```bash
git clone https://github.com/tmux-plugins/tpm ~/.config/tmux/plugins/tpm
```

Then inside tmux, press `prefix + I` to install plugins.

## Full tmux.conf

```
set -g default-terminal "tmux-256color"
set -ag terminal-overrides ",xterm-256color:RGB"

# Pane Navigation (vim-style, no prefix)
bind -n C-w switch-client -T pane-nav
bind -T pane-nav h select-pane -L
bind -T pane-nav j select-pane -D
bind -T pane-nav k select-pane -U
bind -T pane-nav l select-pane -R

set -g base-index 1
setw -g pane-base-index 1
set -g renumber-windows on
set -g status-position top

setw -g mode-keys vi
bind P paste-buffer
bind-key -T copy-mode-vi v send-keys -X begin-selection
bind-key -T copy-mode-vi y send-keys -X copy-pipe-and-cancel "pbcopy"
bind-key -T copy-mode-vi r send-keys -X rectangle-toggle
bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-pipe-and-cancel "pbcopy"
set -s escape-time 0

set -g mouse on

# Prefix
set -g prefix C-q
set -g prefix2 C-Space

# Pane splitting (keep current path)
bind | split-window -h -c "#{pane_current_path}"
bind - split-window -v -c "#{pane_current_path}"

# Session switching by position
bind 1 run-shell "tmux switch-client -t $(tmux list-sessions -F '#S' | sed -n '1p')"
bind 2 run-shell "tmux switch-client -t $(tmux list-sessions -F '#S' | sed -n '2p')"
bind 3 run-shell "tmux switch-client -t $(tmux list-sessions -F '#S' | sed -n '3p')"
bind 4 run-shell "tmux switch-client -t $(tmux list-sessions -F '#S' | sed -n '4p')"
bind 5 run-shell "tmux switch-client -t $(tmux list-sessions -F '#S' | sed -n '5p')"
bind 6 run-shell "tmux switch-client -t $(tmux list-sessions -F '#S' | sed -n '6p')"
bind 7 run-shell "tmux switch-client -t $(tmux list-sessions -F '#S' | sed -n '7p')"
bind 8 run-shell "tmux switch-client -t $(tmux list-sessions -F '#S' | sed -n '8p')"
bind 9 run-shell "tmux switch-client -t $(tmux list-sessions -F '#S' | sed -n '9p')"

bind r switch-client -T reload-table
bind -T reload-table l source-file ~/.config/tmux/tmux.conf \; display "Config reloaded"

bind l switch-client -t hive
```

## Hotkeys

### Prefix
| Key | Action |
|-----|--------|
| `C-q` | Primary prefix |
| `C-Space` | Secondary prefix |

### Panes
| Key | Action |
|-----|--------|
| `C-w h/j/k/l` | Navigate panes (no prefix) |
| `C-q \|` | Split horizontal (keeps path) |
| `C-q -` | Split vertical (keeps path) |

### Sessions & Windows
| Key | Action |
|-----|--------|
| `C-q 1-9` | Switch to nth session by position |
| `C-q l` | Switch to hive session |
| `C-q c` | New window |

### Config
| Key | Action |
|-----|--------|
| `C-q r l` | Reload tmux config |

### Copy Mode
| Key | Action |
|-----|--------|
| `C-q [` | Enter copy mode |
| `v` | Begin selection |
| `y` | Copy to system clipboard |
| Mouse drag | Copy to system clipboard |
| `C-q P` | Paste buffer |

### Session Persistence
| Key | Action |
|-----|--------|
| `C-q C-s` | Save session manually |
| `C-q C-r` | Restore session manually |

Auto-saves every 15 minutes via tmux-continuum. Restores automatically on tmux start.

## Zsh Aliases

```zsh
alias hv='tmux new-session -As hive hive'   # attach/create hive session running hive
alias t='tmux new-session -As $(basename $PWD)'  # attach/create session named after cwd
```

## Notes

- The **left side of the status bar shows the session name**, not a window. Don't
  mistake it for a window tab.
- Always start hive with `hv` so the session is named `hive` — required for
  `C-q l` to switch to it.
- TPM plugins are installed to `~/.config/tmux/plugins/` and should be gitignored.
- tokyo-night theme requires bash 5+ — verify with `which bash` after installing.
