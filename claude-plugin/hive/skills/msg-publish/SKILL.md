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

Output: JSON confirmation line with status, resolved topics, and sender.

Over MCP (Hive Desktop's `hive-desktop` server, or `hive mcp serve --toolset messages`) the same operation is the `message_create` tool. In a Hive chat the sender is the chat's id.

## Commands

### Publish a Message

```bash
hive msg create --topics <topic> --payload "message content"
```

**Examples:**
```bash
# Send to specific agent's inbox
hive msg create --topics agent.abc123.inbox --payload "Task completed, ready for review"

# Send to multiple topics
hive msg create --topics agent.123.inbox --topics agent.456.inbox --payload "Important update"
```

### Publish a File or Command Output

`--payload` takes the whole message as one argument, so read a file or a command's output into it:

```bash
hive msg create --topics agent.abc.inbox --payload "$(cat handoff-context.md)"
```

`hive msg pub` is an alias of `hive msg create` and takes the same flags.

### Broadcast with Wildcards

```bash
# Broadcast to all agent inboxes
hive msg create --topics "agent.*.inbox" --payload "System maintenance in 5 minutes"
```

Wildcard `*` matches any characters within a segment. Topics are expanded at publish time.

## Topic Patterns

### Standard Inbox Format

Agent inboxes follow: `agent.<session-id>.inbox`

```bash
# Find session IDs
hive ls

# Send to specific agent
hive msg create --topics agent.abc123.inbox --payload "Message"
```

### Custom Topics

Create any topic name for specific use cases:

```bash
hive msg create --topics deploy.started --payload "Deployment initiated"
hive msg create --topics build.events --payload "Test suite running"
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
hive msg create --topics agent.xyz789.inbox --payload "Auth implementation complete. See PR #123. Tests passing."
```

### Notify Multiple Topics

```bash
hive msg create \
  --topics agent.abc.inbox \
  --topics agent.xyz.inbox \
  --topics coordinator.updates \
  --payload "Feature X ready for review"
```

### Send Command Output

```bash
hive msg create --topics test.results --payload "$(go test -v ./... 2>&1)"
```

### Override Sender ID

```bash
hive msg create --topics test.channel --sender "custom-agent-id" --payload "Test message"
```

## Additional Resources

For troubleshooting, advanced patterns, and coordination workflows, see:
- **`references/troubleshooting.md`** - Common issues and solutions

## Related Skills

- `/hive:inbox` - Check inbox for messages
- `/hive:wait` - Wait for messages with timeout
- `/hive:session-info` - Get session details and inbox topic
