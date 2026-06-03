---
icon: lucide/rocket
---

# Getting Started

Install hive, configure your environment, and launch your first AI agent session in under five minutes.

## Installation

### Prerequisites

- **Git** — available in your PATH
- **tmux** — required for session management and status monitoring

!!! note "tmux is required"
    Hive uses tmux for session management, agent monitoring, and preview panes. Install it before proceeding — hive will not function without it.

### Homebrew

```bash
brew install tmux
brew install colonyops/tap/hive
```

### GitHub Release

Pre-built binaries for macOS and Linux are available on the [GitHub Releases](https://github.com/colonyops/hive/releases) page. Download the appropriate archive for your platform, extract it, and place the binary on your PATH.

### Go Install

```bash
go install github.com/colonyops/hive@latest
```

### Mise

Install via [mise](https://mise.jdx.dev/) using the GitHub backend:

```bash
mise use -g github:colonyops/hive
```

### Verify

```bash
hive --version
hive doctor        # Check configuration and environment
hive config        # Dump resolved configuration as JSON
```

## Quick Start

### 1. Run the setup wizard

```bash
hive init
```

The interactive wizard:

- Detects installed AI agents (Claude, Codex, OpenCode, and others) and writes `~/.config/hive/config.yaml` with your preferred default
- Appends `alias hv='tmux new-session -As hive hive'` to your shell rc (`.zshrc`, `.bashrc`, or `config.fish`)
- Appends `bind-key h switch-client -t hive` to your `~/.tmux.conf` (or `$XDG_CONFIG_HOME/tmux/tmux.conf`)

Type a workspace path to complete setup (tab or `→` to autocomplete directories), then follow the remaining prompts.

### 2. Launch

```bash
hv
```

Hive auto-detects tmux, bundles its own session management scripts, and registers default keybindings — no other config needed. Press `n` to create sessions, `enter` to open them, and `:` for the command palette.

### Manual setup (alternative)

If you prefer to configure things by hand instead of using the wizard:

**Shell alias** — add to your `.bashrc` / `.zshrc`:

```bash
alias hv='tmux new-session -As hive hive'
```

This runs hive inside a dedicated tmux session called `hive`. If the session already exists, it reattaches.

**tmux keybinding** — add to `~/.tmux.conf`:

```tmux
bind l switch-client -t hive
```

Now `<prefix> l` returns you to hive from any agent session.

**Config file** — tell hive where your repos live:

```yaml
# ~/.config/hive/config.yaml
workspaces:
  - ~/projects
  - ~/work
```

!!! info
    Without `workspaces`, you can still create sessions from the CLI by running `hive new` from within a git repository:

    ```bash
    cd ~/projects/my-app
    hive new Fix Auth Bug
    ```

## Next Steps

- [Sessions](sessions.md) — How sessions, agents, and lifecycle work
- [Task Tracking](task-tracking.md) — Built-in epics and tasks for multi-agent coordination
- [Context](context.md) — Shared storage and the review tool
- [Messaging](messaging.md) — Inter-agent pub/sub communication
- [Todos](todos.md) <span class="hive-experimental-icon" title="Experimental" role="img" aria-label="Experimental"></span> — Operator todo lifecycle and CLI usage
