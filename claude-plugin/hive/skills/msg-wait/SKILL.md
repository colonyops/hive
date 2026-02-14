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

The `hive msg sub --wait` command polls the specified topic every 500ms until:
- A message arrives (success, exit 0)
- The timeout is reached (failure, exit 1)

Default timeout is 30 seconds. Messages are automatically acknowledged unless `--peek` is used.

## Commands

### Wait for Single Message

```bash
hive msg sub --wait --topic <topic>
```

**Examples:**
```bash
# Wait for handoff message (30s timeout)
hive msg sub --wait --topic agent.abc.inbox

# Wait for build completion
hive msg sub --wait --topic build.main.status

# Wait for acknowledgment
hive msg sub --wait --topic agent.xyz.inbox.ack
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

### Wait Without Acknowledging

```bash
hive msg sub --wait --topic <topic> --peek
```

Monitor without consuming messages. Useful when another agent is the primary handler.

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
hive msg pub --topic agent.bob.inbox "Feature X ready. Branch: feat/x"
hive msg sub --wait --topic agent.bob.inbox.ack --timeout 2m
```

### Handle Request-Response

```bash
# Send request
hive msg pub --topic coordinator.requests "Need assignment: task-type-X"

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
