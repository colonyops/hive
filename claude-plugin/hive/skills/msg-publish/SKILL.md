---
name: publish
description: This skill should be used when the user asks to "send message to agent X", "publish to topic", "broadcast to all agents", "notify other sessions", "tell agent Y that...", or needs to send inter-agent messages, hand off work, or broadcast notifications across hive sessions.
compatibility: claude, opencode
---

# Publish - Send Inter-Agent Messages

Send messages to other agents, broadcast to multiple sessions, or publish notifications to topic-based channels.

## When to Use

- Handing off work to another agent
- Notifying other sessions of events or status
- Broadcasting updates to multiple agents
- Sending results or data to waiting agents
- Coordinating distributed work across sessions

## How It Works

Messages are published to named topics. The sender is auto-detected from the current hive session context. Each message includes sender ID, timestamp, content, and topic(s).

## Commands

### Publish Direct Message

```bash
hive msg pub --topic <topic> "message content"
```

**Examples:**
```bash
# Send to specific agent's inbox
hive msg pub --topic agent.abc123.inbox "Task completed, ready for review"

# Send to multiple topics
hive msg pub -t agent.123.inbox -t agent.456.inbox "Important update"
```

### Publish from Stdin

```bash
echo "Status update" | hive msg pub --topic notifications
cat report.txt | hive msg pub --topic reports
```

### Publish from File

```bash
hive msg pub --topic agent.abc.inbox -f handoff-context.md
```

### Broadcast with Wildcards

```bash
# Broadcast to all agent inboxes
hive msg pub -t "agent.*.inbox" "System maintenance in 5 minutes"
```

Wildcard `*` matches any characters within a segment. Topics are expanded at publish time.

## Topic Patterns

### Standard Inbox Format

Agent inboxes follow: `agent.<session-id>.inbox`

```bash
# Find session IDs
hive ls

# Send to specific agent
hive msg pub --topic agent.abc123.inbox "Message"
```

### Custom Topics

Create any topic name for specific use cases:

```bash
hive msg pub --topic deploy.started "Deployment initiated"
hive msg pub --topic build.events "Test suite running"
```

### Topic Naming Conventions

- `agent.<id>.inbox` - Direct messages to agents
- `<domain>.<event>` - Event notifications (build.started, test.failed)
- `<feature>.<channel>` - Feature-specific channels

Avoid generic names, special characters besides `.`, `-`, `_`.

## Common Workflows

### Hand Off Work

```bash
# Complete work and notify next agent
hive msg pub --topic agent.xyz789.inbox "Auth implementation complete. See PR #123. Tests passing."
```

### Notify Multiple Topics

```bash
hive msg pub \
  -t agent.abc.inbox \
  -t agent.xyz.inbox \
  -t coordinator.updates \
  "Feature X ready for review"
```

### Send Command Output

```bash
go test -v ./... | hive msg pub --topic test.results
```

### Override Sender ID

```bash
hive msg pub --topic test.channel --sender "custom-agent-id" "Test message"
```

## Additional Resources

For troubleshooting, advanced patterns, and coordination workflows, see:
- **`references/troubleshooting.md`** - Common issues and solutions

## Related Skills

- `/hive:inbox` - Check inbox for messages
- `/hive:wait` - Wait for messages with timeout
- `/hive:session-info` - Get session details and inbox topic
