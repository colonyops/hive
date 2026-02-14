---
icon: lucide/plug
---

# Claude Code Plugin

Hive provides a Claude Code plugin that gives your agents messaging, session awareness, and coordination capabilities. There are two plugins — install one or both depending on your needs.

## hive

The core plugin. Adds slash commands and skills for inter-agent messaging and session management.

```bash
claude plugin add github:colonyops/hive/claude-plugin/hive
```

### Commands

| Command | Description |
| --- | --- |
| `/hive:coordinate` | Discover other sessions, send messages, and hand off work between agents |
| `/hive:inbox` | Check inbox for unread inter-agent messages |

### Skills

Skills are loaded automatically when the agent detects relevant context — no slash command needed.

| Skill | What it teaches the agent |
| --- | --- |
| `config` | How to configure hive (rules, spawn commands, keybindings, user commands) |
| `inbox` | Reading messages, peeking without marking as read, viewing history |
| `publish` | Sending messages to specific agents, broadcasting, topic naming conventions |
| `wait` | Blocking until a message arrives, timeouts, synchronization patterns |
| `session-info` | Retrieving current session ID, inbox topic, and session state |

## hive-hooks

!!! warning "Experimental"
    This plugin is experimental and may change in future releases.

Automates session lifecycle events. Currently provides a single hook that checks for unread inbox messages when a Claude session starts, and injects a system message if any are found.

```bash
claude plugin add github:colonyops/hive/claude-plugin/hive-hooks
```

### Hooks

| Event | Behavior |
| --- | --- |
| `SessionStart` | Peeks at the inbox and notifies the agent of unread messages with a preview |

The hook runs silently — if hive isn't installed or the session isn't inside a hive workspace, it exits without error.
