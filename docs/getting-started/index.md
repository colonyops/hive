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
```

## Quick Start

### 1. Run the setup wizard

The easiest way to get started is to run the interactive setup wizard:

```bash
hive install
```

This will:

- Add the `hv` shell alias to your shell config
- Verify your AI agent tools (claude, codex) are available in PATH

After running install, source your shell config or restart your terminal.

??? info "Manual setup"
    If you prefer to set things up manually, add this alias to your `.bashrc` / `.zshrc`:

    ```bash
    alias hv='tmux new-session -As hive hive'
    ```

    This runs hive inside a dedicated tmux session called `hive`. If the session already exists, it reattaches.

Hive works best when running inside tmux. When you create a session, hive spawns a tmux session for the agent — if hive itself is also a tmux session, you can seamlessly switch between hive and your agents without leaving tmux. Hive becomes your home base: open an agent, do some work, jump back to hive, spin up another.

### 2. Add a tmux keybinding to jump back

Add to your `~/.tmux.conf`:

```tmux
bind l switch-client -t hive
```

Now `<prefix> l` returns you to hive from any agent session.

### 3. Configure workspaces (optional)

To create sessions from the TUI with `n`, tell hive where your repos live:

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

### 4. Launch

```bash
hv
```

Hive auto-detects tmux, bundles its own session management scripts, and registers default keybindings — no other config needed. Press `n` to create sessions, `enter` to open them, and `:` for the command palette.

## Next Steps

- [Sessions](sessions.md) — How sessions, agents, and lifecycle work
- [Context](context.md) — Shared storage and the review tool
- [Messaging](messaging.md) — Inter-agent pub/sub communication
