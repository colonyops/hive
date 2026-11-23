---
name: wait
description: This skill should be used when the user asks to "wait for message from agent X", "block until response", "wait for handoff", "synchronize with other agents", "wait for acknowledgment", or needs to block execution until messages arrive on specific topics with configurable timeout.
compatibility: claude, opencode
---

# Wait - Block Until Messages Arrive

Wait for messages on specific topics, enabling synchronization between agents and coordinated handoff workflows.

## When to Use

- Waiting for another agent to complete work
- Coordinating handoff between agents
- Blocking until a response or acknowledgment arrives
- Synchronizing at specific points in distributed workflows

## How It Works

`hive message list --topic <topic> --wait <seconds>` polls the topic every 500ms until:
- A message published after the call started arrives (prints it, exit 0)
- The wait ends (prints `{"status":"timeout",...}`, exit 0)

The wait is at most 86400 seconds (24h). Over MCP the same call is the `message_list` tool with `wait`; keep it short there, because an MCP client may give up on a long call. Reading marks nothing read: mark a message read with `hive message-read put --message-ids <id>`.

### Older Spelling: `hive msg sub --wait`

`hive msg sub --wait` still works. It takes a Go duration (`--timeout 2m`, default 24h), exits 1 on timeout, and can mark the message read with `--ack`, so it suits a shell `if`. The examples below use it.

## Commands

### Wait for Single Message

```bash
hive msg sub --wait --topic <topic>
```

**Examples:**
```bash
# Wait for handoff message (24h default timeout)
hive msg sub --wait --topic agent.abc.inbox

# Wait for build completion
hive msg sub --wait --topic build.main.status
```

### Wait with Custom Timeout

```bash
hive msg sub --wait --topic <topic> --timeout <duration>
```

**Timeout format:** `s` (seconds), `m` (minutes), `h` (hours)

```bash
# Short timeout for quick checks
hive msg sub --wait --topic notifications --timeout 5s

# Moderate timeout for typical handoffs
hive msg sub --wait --topic agent.abc.inbox --timeout 2m

# Long timeout for slow operations
hive msg sub --wait --topic build.production --timeout 10m
```

### Wait and Acknowledge

```bash
hive msg sub --wait --topic <topic> --ack
```

Mark the received message as read so it won't appear in unread queries.

### Wait with Wildcard Topics

```bash
# Wait for any agent to respond
hive msg sub --wait --topic "agent.*.response"

# Wait for any build event
hive msg sub --wait --topic "build.*.status"
```

### Monitor Continuously

Use `--listen` mode for continuous message monitoring (outputs ALL messages until timeout):

```bash
hive msg sub --listen --topic notifications --timeout 1h
```

**Key difference:** `--wait` returns after ONE message. `--listen` continues polling and outputs ALL messages.

### Inbox Wait Shorthand

```bash
hive msg inbox --wait
hive msg inbox --wait --timeout 2m --ack
```

Equivalent to `hive msg sub --wait --topic agent.<id>.inbox` but auto-detects the inbox topic.

## Output Format

All output is JSON Lines on stdout. On timeout, a JSON status line is printed:
```json
{"status":"timeout","topic":"agent.abc.inbox","duration":"30s"}
```
Exit code is 1 on timeout.

## Timeout Guidelines

| Duration | Use Case |
|----------|----------|
| 5-30s | Quick handoffs between active agents |
| 1-5m | Normal agent handoffs, human review |
| 10m-1h | Build/test operations, background processing |
| 1-24h | Overnight jobs, asynchronous collaboration |

## Common Workflows

### Coordinate Agent Handoff

```bash
# Complete work, notify, and wait for acknowledgment
hive msg pub --topic agent.bob.inbox -m "Feature X ready. Branch: feat/x"
hive msg sub --wait --topic agent.bob.inbox.ack --timeout 2m
```

### Handle Request-Response

```bash
# Send request
hive msg pub --topic coordinator.requests -m "Need assignment: task-type-X"

# Wait for response
hive msg sub --wait --topic agent.myself.inbox --timeout 1m
```

### Handle Timeouts

```bash
if hive msg sub --wait --topic agent.bob.inbox --timeout 30s; then
    echo "Message received"
else
    echo "Timeout: no message received"
fi
```

## Additional Resources

For advanced patterns and troubleshooting, see:
- **`references/troubleshooting.md`** - Common issues and solutions

## Related Skills

- `/hive:inbox` - Check inbox for messages
- `/hive:publish` - Send messages to other agents
- `/hive:session-info` - Get session details and inbox topic
