---
icon: fontawesome/brands/hive
---

# Hive

Hive manages isolated git sessions for running AI agents in parallel. Instead of manually managing git worktrees or directories, hive handles cloning, recycling, and spawning terminal sessions with your preferred AI tool (Claude, Aider, Codex). Each session is a complete git clone with its own terminal environment, tracked through a lifecycle (active → recycled → deleted) and reusable to save time and disk space.

## Key Features

- **Session Management** — Create, recycle, and prune isolated git clones
- **Terminal Integration** — Real-time status monitoring of AI agents in tmux (works out of the box)
- **Inter-agent Messaging** — Pub/sub communication between sessions
- **Context Sharing** — Shared storage per repository via `.hive` symlinks
- **Custom Keybindings** — Bind keys to user-defined or system commands
- **Command Palette** — Vim-style command palette for custom commands (`:` key)

## Installation

Download the latest binary from [GitHub Releases](https://github.com/colonyops/hive/releases) and place it on your PATH.

## Status Indicators

The TUI shows real-time agent status:

| Indicator | Color            | Meaning                         |
| --------- | ---------------- | ------------------------------- |
| `[●]`     | Green (animated) | Agent actively working          |
| `[!]`     | Yellow           | Agent needs approval/permission |
| `[>]`     | Cyan             | Agent ready for input           |
| `[?]`     | Dim              | Terminal session not found      |
| `[○]`     | Gray             | Session recycled                |

## Next Steps

- [Getting Started](getting-started/) — Quick start and first session
- [Sessions](getting-started/sessions.md) — How sessions, agents, and lifecycle work
- [Configuration](configuration/) — Config file, rules, templates, and options

---

<small>LLM-friendly: [llms.txt](../llms.txt) | [llms-full.txt](../llms-full.txt)</small>
