---
name: hive:inbox
description: Check your inbox for inter-agent messages. Use when you need to read messages, check for unread messages, or review communication from other agents.
compatibility: claude, opencode
---

# Inbox - Read Inter-Agent Messages

Check and read messages sent to your session's inbox from other agents or sessions.

## When to Use

Use this skill when:
- Another agent mentions sending you a message
- You need to check for pending messages
- You want to review recent inter-agent communications
- Starting work on a task that may have been handed off to you
- Coordinating with other Claude sessions

**Common triggers:**
- "check my inbox"
- "read my messages"
- "any unread messages?"
- "check for new messages"
- "see my inbox"

## How It Works

Each hive session has a unique inbox topic (`agent.<id>.inbox`). Other agents can publish messages to your inbox, and you can read them using this skill.

Messages are automatically marked as read by default when you check your inbox. This prevents re-reading the same messages multiple times.

## Commands

### Read Unread Messages (Default)

```bash
hive msg inbox
```

Shows only unread messages and marks them as read. This is the most common usage.

**Output format:**
- Each message shows timestamp, sender, and content
- Messages are displayed in chronological order
- After reading, messages are marked with `read: true`

### Peek Without Marking as Read

```bash
hive msg inbox --peek
```

View unread messages without marking them as read. Useful when you want to check messages but aren't ready to act on them yet.

**Use cases:**
- Quick check during another task
- Previewing messages before committing to read them
- Debugging message flow

### Read All Messages

```bash
hive msg inbox --all
```

Shows all messages in your inbox, both read and unread. Does NOT mark messages as read.

**Use cases:**
- Reviewing conversation history
- Finding a specific past message
- Auditing communication timeline

### JSON Output

```bash
hive msg inbox --json
```

Returns messages in JSON format for programmatic processing. Can be combined with other flags:
- `hive msg inbox --json` - Unread messages, mark as read
- `hive msg inbox --peek --json` - Unread messages, don't mark as read
- `hive msg inbox --all --json` - All messages, don't mark as read

## Message Format

Messages contain:
- `id`: Unique message identifier
- `sender`: Who sent the message (agent ID or session name)
- `timestamp`: When the message was sent
- `content`: The message text
- `read`: Whether you've read the message (boolean)
- `topic`: The inbox topic (usually `agent.<id>.inbox`)

## Auto-Acknowledgment System

When you read messages from your inbox, the hive system automatically acknowledges receipt. This is built on four phases:

1. **Foundation**: Messages store read/unread state
2. **Read Tracking**: `hive msg inbox` updates read status
3. **Acknowledgment Channels**: Senders can subscribe to ack topics
4. **Confirmation Loop**: Acknowledgments are published when messages are read

### How Acknowledgments Work

When an agent sends you a message with `hive msg pub agent.123.inbox "message"`, they can optionally wait for acknowledgment:

```bash
# Sender subscribes to acknowledgment topic
hive msg sub agent.123.inbox.ack
```

When you read the message with `hive msg inbox`, the system automatically:
1. Marks the message as read
2. Publishes an acknowledgment to `<topic>.ack`
3. Includes the message ID and read timestamp

The sender receives confirmation that you've read their message.

## Common Workflows

### Basic Message Check

```bash
# Check for new messages
hive msg inbox

# If there are messages, read and respond if needed
# Messages are automatically marked as read
```

### Coordinated Handoff

When another agent hands off work to you:

```bash
# Agent A publishes to your inbox:
# "Task X is ready. See issue hive-123."

# You check your inbox
hive msg inbox

# Read the task details
bd show hive-123

# Start working on the task
```

### Review Past Communication

```bash
# See all messages (read and unread)
hive msg inbox --all

# Find specific conversation
hive msg inbox --all | grep "handoff"
```

## Troubleshooting

### No Messages Showing

**Problem:** Running `hive msg inbox` shows no messages

**Solutions:**
- Check if you're in the correct session: `hive session ls`
- Verify your inbox topic: Your inbox is `agent.<session-id>.inbox`
- Check if messages exist: `hive msg inbox --all`

### Messages Not Marked as Read

**Problem:** Messages keep showing as unread

**Solutions:**
- Ensure you're NOT using `--peek` or `--all` flags
- Use default command: `hive msg inbox`
- Check message file permissions in `$XDG_DATA_HOME/hive/messages/topics/`

### Can't Find Specific Message

**Problem:** Looking for a message that was sent earlier

**Solutions:**
- Use `--all` to see full history: `hive msg inbox --all`
- Search with grep: `hive msg inbox --all | grep "keyword"`
- Check with JSON: `hive msg inbox --all --json | jq '.[] | select(.sender == "agent-123")'`

## Related Skills

- `/hive:publish` - Send messages to other agents
- `/hive:wait` - Wait for specific messages with timeout
- `/hive:session-info` - Get your current session details

## Tips

**Be Proactive:**
- Check inbox at start of new work sessions
- Review messages when context switching between tasks

**Stay Organized:**
- Read messages when you can act on them
- Use `--peek` only for quick checks
- Use `--all` to review conversation threads

**Coordinate Effectively:**
- Acknowledge handoffs by starting the referenced work
- Send responses using `/hive:publish` when appropriate
- Keep message history for context on long-running work
