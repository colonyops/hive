---
icon: fontawesome/brands/hive
---

<div align="center">
  <img src="assets/favicon.svg" alt="Hive" width="100">
  <h1 style="margin-top: 0.5em; margin-bottom: 0.5em;">Hive</h1>
</div>

Hive manages isolated git sessions for running AI agents in parallel. Instead of manually managing git worktrees or directories, hive handles cloning, recycling, and spawning terminal sessions with your preferred AI tool (Claude, Aider, Codex). Each session is a complete git clone with its own terminal environment, tracked through a lifecycle (active → recycled → deleted) and reusable to save time and disk space.

## Key Features

<div class="grid cards" markdown>

- :lucide-layers: __Session Management__

    ---

    Create, recycle, and prune isolated git clones

- :lucide-terminal: __Terminal Integration__

    ---

    Real-time status monitoring of AI agents in tmux (works out of the box)

- :lucide-message-circle: __Inter-agent Messaging__

    ---

    Pub/sub communication between sessions

- :lucide-folder-symlink: __Context Sharing__

    ---

    Shared storage per repository via `.hive` symlinks

- :lucide-keyboard: __Custom Keybindings__

    ---

    Bind keys to user-defined or system commands

- :lucide-command: __Command Palette__

    ---

    Vim-style command palette for custom commands (`:` key)

</div>

## Installation

Download the latest binary from [GitHub Releases](https://github.com/colonyops/hive/releases) and place it on your PATH.

!!! tip "Claude Code Plugin"
    Hive provides a [Claude Code plugin](https://github.com/colonyops/hive/tree/main/claude-plugin/hive) for inter-agent messaging, session management, and workflow coordination. Install it with:

    ```bash
    claude plugin add github:colonyops/hive/claude-plugin/hive
    ```

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

<div class="grid cards" markdown>

- :lucide-rocket: __[Getting Started](getting-started/)__

    ---

    Quick start and first session

- :lucide-box: __[Sessions](getting-started/sessions.md)__

    ---

    How sessions, agents, and lifecycle work

- :lucide-settings: __[Configuration](configuration/)__

    ---

    Config file, rules, templates, and options

</div>

---

<small>LLM-friendly: [llms.txt](../llms.txt) | [llms-full.txt](../llms-full.txt)</small>
