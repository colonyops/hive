---
name: inbox
description: This skill should be used when the user asks to "check my inbox", "read my messages", "any unread messages?", "check for new messages", "see my inbox", or needs to read inter-agent messages from other hive sessions. Provides guidance on reading, filtering, and managing inbox messages.
compatibility: claude, opencode
---

# Inbox - Read Inter-Agent Messages

Check and read messages sent to this session's inbox from other agents or sessions.

## When to Use

- Another agent mentions sending a message
- Starting work on a task that may have been handed off
- Coordinating with other Claude sessions
- Checking for pending messages or recent communications

## How It Works

Each hive session has a unique inbox topic (`agent.<id>.inbox`). Other agents publish messages to this inbox, readable via the commands below.

By default, messages are NOT marked as read. Use `--ack` to acknowledge messages.

## Commands

### Read Unread Messages (Default)

```bash
hive msg inbox
```

Shows unread messages without marking them as read.

### Read and Acknowledge

```bash
hive msg inbox --ack
```

Shows unread messages and marks them as read so they won't appear again.

### Read All Messages

```bash
hive msg inbox --all
```

Shows all messages (read and unread).

### Specify Session Explicitly

```bash
hive msg inbox --session <id|name>
```

Overrides auto-detection from working directory. Useful when running outside a session directory.

### Wait for a Message

```bash
hive msg inbox --wait
hive msg inbox --wait --timeout 2m
```

Blocks until a message arrives. Default timeout is 24h for wait mode.

### Poll for New Messages

```bash
hive msg inbox --listen --timeout 30s
```

Continuously polls and outputs new messages until timeout.

### Limit Results

```bash
hive msg inbox --tail 5
```

Returns only the last N unread messages.

## Output Format

All output is JSON Lines (one JSON object per line) on stdout. Fields:
- `id` - Unique message identifier
- `topic` - The inbox topic (`agent.<id>.inbox`)
- `payload` - The message text
- `sender` - Who sent the message (session ID or custom sender)
- `session_id` - Sender's session ID (if auto-detected)
- `created_at` - ISO 8601 timestamp

On timeout (`--listen`/`--wait`), a JSON status line is printed and exit code is 1.

## Common Workflows

### Basic Message Check

```bash
hive msg inbox
```

Read and act on any unread messages.

### Handle Coordinated Handoff

When another agent hands off work:

```bash
# Check inbox for handoff message
hive msg inbox

# Read referenced task details
bd show <issue-id>
```

### Review Message History

```bash
hive msg inbox --all
```

## Additional Resources

For troubleshooting and advanced usage patterns, see:
- **`references/troubleshooting.md`** - Common issues and solutions

## Related Skills

- `/hive:publish` - Send messages to other agents
- `/hive:wait` - Wait for specific messages with timeout
- `/hive:session-info` - Get current session details and inbox topic
