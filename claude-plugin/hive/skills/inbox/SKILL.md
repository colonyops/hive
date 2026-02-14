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

Messages are automatically marked as read when checked with the default command. This prevents re-reading the same messages.

## Commands

### Read Unread Messages (Default)

```bash
hive msg inbox
```

Shows only unread messages and marks them as read. Most common usage.

### Peek Without Marking as Read

```bash
hive msg inbox --peek
```

View unread messages without marking them as read. Useful for quick checks during other work.

### Read All Messages

```bash
hive msg inbox --all
```

Shows all messages (read and unread). Does NOT mark messages as read.

## Message Format

Messages contain:
- `id` - Unique message identifier
- `sender` - Who sent the message (agent ID or session name)
- `timestamp` - When the message was sent
- `content` - The message text
- `read` - Whether the message has been read
- `topic` - The inbox topic (usually `agent.<id>.inbox`)

## Common Workflows

### Basic Message Check

```bash
hive msg inbox
```

Read and act on any unread messages. Messages are automatically marked as read.

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

## Auto-Acknowledgment

When messages are read with `hive msg inbox`, the system automatically:
1. Marks the message as read
2. Publishes an acknowledgment to `<topic>.ack`
3. Includes the message ID and read timestamp

Senders can subscribe to acknowledgment topics to confirm receipt.

## Additional Resources

For troubleshooting and advanced usage patterns, see:
- **`references/troubleshooting.md`** - Common issues and solutions

## Related Skills

- `/hive:publish` - Send messages to other agents
- `/hive:wait` - Wait for specific messages with timeout
- `/hive:session-info` - Get current session details and inbox topic
