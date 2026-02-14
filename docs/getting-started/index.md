# Getting Started

## Quick Start

**Prerequisites:** Git and tmux installed.

### 1. Set up a shell alias

Hive works best when running inside tmux. When you create a session, hive spawns a tmux session for the agent — if hive itself is also a tmux session, you can seamlessly switch between hive and your agents without leaving tmux. Hive becomes your home base: open an agent, do some work, jump back to hive, spin up another.

Add to your `.bashrc` / `.zshrc`:

```bash
alias hv='tmux new-session -As hive hive'
```

This runs hive inside a dedicated tmux session called `hive`. If the session already exists, it reattaches.

### 2. Add a tmux keybinding to jump back

Add to your `~/.tmux.conf`:

```tmux
bind l switch-client -t hive
```

Now `<prefix> l` returns you to hive from any agent session.

### 3. Configure repo directories (optional)

To create sessions from the TUI with `n`, tell hive where your repos live:

```yaml
# ~/.config/hive/config.yaml
repo_dirs:
  - ~/projects
  - ~/work
```

Without this, you can still create sessions from the CLI:

```bash
cd ~/projects/my-app
hive new Fix Auth Bug
```

### 4. Launch

```bash
hv
```

Hive auto-detects tmux, bundles its own session management scripts, and registers default keybindings — no other config needed. Press `n` to create sessions, `enter` to open them, and `:` for the command palette.

## Dependencies

- Git (available in PATH or configured via `git_path`)
- tmux (required — provides session management and status monitoring)

## Next Steps

- [Sessions](sessions.md) — How sessions, agents, and lifecycle work
- [Context](context.md) — Shared storage and the review tool
- [Messaging](messaging.md) — Inter-agent pub/sub communication
