---
name: session-info
description: This skill should be used when the user asks "what's my session ID?", "show my inbox topic", "get session info", "what session am I in?", "my agent ID", or needs to retrieve current hive session details for messaging coordination or debugging.
compatibility: claude, opencode
---

# Session Info - Get Current Session Details

Display information about the current hive session, including session ID, inbox topic, and state.

## Terminology

- **Session** - An isolated git clone + terminal environment managed by hive
- **Session ID** - A unique 6-character identifier (e.g., `26kj0c`)
- **Agent** - The AI tool (Claude, Aider, etc.) running within the session
- **Inbox** - A message topic for receiving messages: `agent.<session-id>.inbox`

## When to Use

- Need to know current session ID
- Another agent asks for inbox topic
- Setting up inter-agent coordination
- Debugging messaging issues
- Verifying correct session context

## Commands

### Get Session Info (Human-Readable)

```bash
hive session info
```

**Example output:**
```
Session ID:  26kj0c
Name:        claude-plugin
Repository:  hive
Inbox:       agent.26kj0c.inbox
Path:        /Users/hayden/.local/share/hive/repos/hive-claude-plugin-26kj0c
State:       active
```

### Get Session Info (JSON)

```bash
hive session info --json
```

**Example output:**
```json
{
  "id": "26kj0c",
  "name": "claude-plugin",
  "repository": "hive",
  "inbox": "agent.26kj0c.inbox",
  "path": "/Users/hayden/.local/share/hive/repos/hive-claude-plugin-26kj0c",
  "state": "active"
}
```

## Output Fields

| Field | Description | Example |
|-------|-------------|---------|
| `id` | Unique 6-char session identifier | `26kj0c` |
| `name` | Human-readable session name | `claude-plugin` |
| `repository` | Git repository name | `hive` |
| `inbox` | Full inbox topic | `agent.26kj0c.inbox` |
| `path` | Absolute path to session directory | `/Users/...` |
| `state` | Session state | `active`, `recycled`, `corrupted` |

## Session States

- **active** - Normal, working session ready for use
- **recycled** - Session has been stopped and cleared
- **corrupted** - Session directory or files are damaged

## Common Workflows

### Share Inbox with Another Agent

```bash
hive session info --json | jq -r '.inbox'
# Output: agent.26kj0c.inbox
```

### Extract Session ID for Scripting

```bash
SESSION_ID=$(hive session info --json | jq -r '.id')
INBOX=$(hive session info --json | jq -r '.inbox')
```

### Verify Session Before Messaging

```bash
# Check session context is correct
hive session info

# Then check inbox
hive msg inbox
```

## How Session Detection Works

The command detects the current session from the working directory. Sessions are identified by:
1. Working directory path within a hive session directory
2. Session directory pattern: `$XDG_DATA_HOME/hive/repos/<repo>-<session-id>`
3. `.hive-session` file in the directory

If not in a hive session directory, the command fails with an error.

## Additional Resources

For troubleshooting and advanced usage, see:
- **`references/troubleshooting.md`** - Common issues and solutions

## Related Skills

- `/hive:inbox` - Check inbox for messages
- `/hive:publish` - Send messages to other agents' inboxes
- `/hive:wait` - Wait for messages on inbox
