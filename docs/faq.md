---
icon: lucide/circle-help
---

# FAQ

### What's the difference between a hive session and a tmux session?

- **Hive session**: A git clone + terminal environment managed by hive
- **Tmux session**: A terminal multiplexer session that hosts the hive session

When you create a hive session with tmux integration, hive spawns a tmux session with the same name. The relationship is:

```
Hive Session "fix-bug" (ID: abc123)
  ↓ creates
Tmux Session "fix-bug"
  ↓ contains
Agent (Claude) running in tmux window
```

See [Getting Started](getting-started/index.md) for full terminology.

### Can I run multiple agents in one session?

Yes. Each agent runs in its own tmux window within the session. Configure [agent profiles](configuration/index.md#agents) to define which tools are available, and use `tmux.preview_window_matcher` to tell hive which windows to monitor. The TUI tracks each agent window independently with its own status indicator.

### Why is the inbox format `agent.<id>.inbox` not `session.<id>.inbox`?

The inbox belongs to the agent (AI tool), not the session (container). The `agent.` prefix supports future per-agent addressing using `agent.<session-id>.<agent-name>.inbox`.

See [Messaging](getting-started/messaging.md) for full details on inter-agent communication.

### What's a "recycled" session?

When you're done with a session, you can recycle it instead of deleting it. Recycling:

1. Resets the git repository to a clean state
2. Renames the directory to a recycled pattern
3. Makes it available for reuse

When you create a new session, hive will reuse a recycled session if available, avoiding a fresh clone and saving time.
