<div align="center">

<img src="docs/assets/favicon.svg" alt="Hive" width="80">

# hive

**The command center for your AI colony**

Manage multiple AI agent sessions in isolated git environments with real-time status monitoring.

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&labelColor=1a1b26)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-9ece6a?style=for-the-badge&labelColor=1a1b26)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux-7aa2f7?style=for-the-badge&labelColor=1a1b26)](https://github.com/colonyops/hive)
[![Release](https://img.shields.io/github/v/release/colonyops/hive?style=for-the-badge&color=e0af68&labelColor=1a1b26)](https://github.com/colonyops/hive/releases)

[Documentation](https://colonyops.github.io/hive/) | [Getting Started](https://colonyops.github.io/hive/getting-started/) | [Configuration](https://colonyops.github.io/hive/configuration/) | [Contributing](#contributing)

</div>

---

## Installation

Download the latest binary from [GitHub Releases](https://github.com/colonyops/hive/releases) and place it on your PATH.

## Overview

Hive manages isolated git sessions for running AI agents in parallel. Instead of manually managing git worktrees or directories, hive handles cloning, recycling, and spawning terminal sessions with your preferred AI tool (Claude, Aider, Codex).

Each hive session is a complete git clone in a dedicated directory with its own terminal environment. Sessions are tracked through a lifecycle (active → recycled → deleted) and can be reused, reducing clone time and disk usage.

**Key Features:**

- **Session Management** — Create, recycle, and prune isolated git clones
- **Terminal Integration** — Real-time status monitoring of AI agents in tmux (works out of the box)
- **Inter-agent Messaging** — Pub/sub communication between sessions
- **Context Sharing** — Shared storage per repository via `.hive` symlinks
- **Custom Keybindings** — Bind keys to user-defined or system commands
- **Command Palette** — Vim-style command palette for custom commands (`:` key)

## Quick Start

**Prerequisites:** Git and tmux installed.

```bash
# Add alias to .bashrc/.zshrc
alias hv='tmux new-session -As hive hive'

# Add to ~/.tmux.conf to jump back to hive
# bind l switch-client -t hive

# Launch
hv
```

Press `n` to create sessions, `enter` to open them, and `:` for the command palette.

See the [Getting Started guide](https://colonyops.github.io/hive/getting-started/) for full setup instructions.

## Status Indicators

| Indicator | Color            | Meaning                         |
| --------- | ---------------- | ------------------------------- |
| `[●]`     | Green (animated) | Agent actively working          |
| `[!]`     | Yellow           | Agent needs approval/permission |
| `[>]`     | Cyan             | Agent ready for input           |
| `[?]`     | Dim              | Terminal session not found      |
| `[○]`     | Gray             | Session recycled                |

## Documentation

Full documentation is available at **[colonyops.github.io/hive](https://colonyops.github.io/hive/)**.

- [Getting Started](https://colonyops.github.io/hive/getting-started/) — Terminology, quick start, first session
- [Configuration](https://colonyops.github.io/hive/configuration/) — Config file, rules, templates, options
- [User Commands](https://colonyops.github.io/hive/configuration/commands/) — User commands and command palette
- [Keybindings](https://colonyops.github.io/hive/configuration/keybindings/) — Key mappings and palette commands
- [Messaging](https://colonyops.github.io/hive/getting-started/messaging/) — Inter-agent pub/sub communication
- [Plugins](https://colonyops.github.io/hive/configuration/plugins/) — Claude, tmux, and other plugins
- [Themes](https://colonyops.github.io/hive/configuration/themes/) — Built-in themes and custom palettes
- [Context & Review](https://colonyops.github.io/hive/getting-started/context/) — Shared context directories and review tool
- [FAQ](https://colonyops.github.io/hive/faq/) — Common questions

## Dependencies

- Git (available in PATH or configured via `git_path`)
- tmux (required — provides session management and status monitoring)

## Contributing

### Prerequisites

- Go `1.25+`
- `git`
- `tmux`
- `task` (https://taskfile.dev)
- `golangci-lint`

### Local Dev Setup

```bash
git clone https://github.com/colonyops/hive.git
cd hive
task run
```

Use `task run` during development. It runs hive with project-local defaults from `Taskfile.yml`:

- `HIVE_CONFIG=./config.dev.yaml`
- `HIVE_DATA_DIR=./.data`
- `HIVE_LOG_FILE=./dev.log`
- `HIVE_LOG_LEVEL=debug`

This isolates contributor testing from your personal/global hive sessions.

### Dev Config vs Global Config

| Mode   | Config Path                  | Data Directory        | Use Case                                |
| ------ | ---------------------------- | --------------------- | --------------------------------------- |
| Dev    | `./config.dev.yaml`          | `./.data`             | Contributing and testing changes safely |
| Global | `~/.config/hive/config.yaml` | `~/.local/share/hive` | Day-to-day hive usage                   |

Run with dev config:

```bash
task run
task run -- new
task run -- doctor
```

Run against your global config (compatibility checks):

```bash
go run *.go --config ~/.config/hive/config.yaml --data-dir ~/.local/share/hive
```

### Common Contributor Commands

```bash
task test
task lint
task check
task build
task validate
```

Before opening a PR, run `task check`. If you changed config behavior, also run `task validate` and `task run -- doctor`.

## Acknowledgments

This project was heavily inspired by [agent-deck](https://github.com/asheshgoplani/agent-deck) by Ashesh Goplani. Several concepts and code patterns were adapted from their work. Thanks to the agent-deck team for open-sourcing their project under the MIT license.

**Disclaimer:** The majority of this codebase was vibe-coded with AI assistance. Use at your own risk.
