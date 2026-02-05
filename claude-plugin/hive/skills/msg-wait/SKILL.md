---
name: wait
description: Wait for messages from other agents with timeout. Use for synchronization points, agent handoff coordination, or blocking until responses arrive.
compatibility: claude, opencode
---

# Wait - Block Until Messages Arrive

Wait for messages on specific topics, enabling synchronization between agents and coordinated handoff workflows.

## When to Use

Use this skill when:
- Waiting for another agent to complete their work
- Coordinating handoff between agents
- Blocking until a response or acknowledgment arrives
- Synchronizing at specific points in distributed workflows
- Polling for status updates or events

**Common triggers:**
- "wait for message from agent X"
- "block until response"
- "wait for handoff"
- "synchronize with other agents"
- "wait for acknowledgment"

## How It Works

The `hive msg sub --wait` command polls the specified topic every 500ms until:
- A message arrives (success)
- The timeout is reached (failure)

By default, the timeout is 30 seconds, but can be adjusted with `--timeout` flag.

When a message arrives, it's automatically acknowledged (marked as read) unless `--peek` is used.

## Commands

### Wait for Single Message

Block until one message arrives on a topic:

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

**Behavior:**
- Polls topic every 500ms
- Returns immediately when message arrives
- Exits with error if timeout reached
- Marks message as read (acknowledged)

### Wait with Custom Timeout

Adjust timeout for longer or shorter waits:

```bash
hive msg sub --wait --topic <topic> --timeout <duration>
```

**Examples:**
```bash
# Short timeout for quick checks
hive msg sub --wait --topic notifications --timeout 5s

# Moderate timeout for typical handoffs
hive msg sub --wait --topic agent.abc.inbox --timeout 2m

# Long timeout for slow operations
hive msg sub --wait --topic build.production --timeout 10m

# Very long timeout for background work
hive msg sub --wait --topic overnight.jobs --timeout 24h
```

**Timeout format:**
- `s` - seconds (5s, 30s, 90s)
- `m` - minutes (1m, 5m, 30m)
- `h` - hours (1h, 12h, 24h)

### Wait Without Acknowledging

Peek at messages without marking them as read:

```bash
hive msg sub --wait --topic <topic> --peek
```

**Use cases:**
- Monitoring without consuming messages
- Checking for messages while another agent is primary handler
- Debugging message flow

### Wait with Wildcard Topics

Wait for messages on any matching topic:

```bash
hive msg sub --wait --topic "pattern.*"
```

**Examples:**
```bash
# Wait for any agent to respond
hive msg sub --wait --topic "agent.*.response"

# Wait for any build event
hive msg sub --wait --topic "build.*.status"
```

## Polling Behavior

The wait command uses polling with these characteristics:

**Polling interval:** 500ms (0.5 seconds)
- Checks topic for new messages every half second
- Low overhead for typical coordination scenarios
- Balance between responsiveness and system load

**Default timeout:** 30s
- Suitable for most agent coordination
- Override with `--timeout` for specific needs

**Exit conditions:**
- **Success (exit 0):** Message received before timeout
- **Failure (exit 1):** Timeout reached with no message

## Use Cases

### Agent Handoff Coordination

**Agent A (completing work):**
```bash
# Complete work
git commit -m "feat: implement feature X"

# Notify agent B
hive msg pub --topic agent.bob.inbox "Feature X ready. Branch: feat/x"

# Wait for acknowledgment
hive msg sub --wait --topic agent.bob.inbox.ack --timeout 2m
```

**Agent B (receiving work):**
```bash
# Check inbox (auto-acknowledges)
hive msg inbox
# > Message from agent.alice: Feature X ready...

# Start work immediately
```

### Long-Running Task Synchronization

Wait for background task to complete:

```bash
# Agent starts long task
hive msg pub --topic build.production "Starting production build" &

# Other agents wait for completion
hive msg sub --wait --topic build.production.complete --timeout 1h
```

### Multi-Agent Coordination

Coordinate multiple agents at synchronization point:

```bash
# Agent 1: Complete phase 1
hive msg pub --topic phase1.complete "Agent 1 done"

# Agent 2: Complete phase 1
hive msg pub --topic phase1.complete "Agent 2 done"

# Coordinator: Wait for both (listen mode, wait for 2 messages)
hive msg sub --listen --topic phase1.complete --timeout 5m
```

### Request-Response Pattern

**Requester:**
```bash
# Send request
hive msg pub --topic coordinator.requests "Need assignment: task-type-X"

# Wait for response
hive msg sub --wait --topic agent.myself.inbox --timeout 1m
```

**Responder:**
```bash
# Monitor requests
hive msg sub --listen --topic coordinator.requests

# Send response
hive msg pub --topic agent.requester.inbox "Assigned: task-123"
```

## Timeout Handling

### Choosing Appropriate Timeouts

**Short timeouts (5-30s):**
- Quick handoffs between active agents
- Real-time coordination
- Fast-fail scenarios

**Medium timeouts (1-5m):**
- Normal agent handoffs
- Wait for human review
- Moderate-speed operations

**Long timeouts (10m-1h):**
- Build and test operations
- Code review wait times
- Background processing

**Very long timeouts (1-24h):**
- Overnight jobs
- Multi-day workflows
- Asynchronous collaboration

### Handling Timeout Failures

The wait command exits with status 1 when timeout is reached:

```bash
# Wait with timeout handling
if hive msg sub --wait --topic agent.bob.inbox --timeout 30s; then
    echo "Message received"
else
    echo "Timeout: no message received"
    # Handle timeout scenario
fi
```

**Timeout strategies:**
- **Retry:** Try waiting again with longer timeout
- **Fallback:** Proceed without waiting for message
- **Alert:** Notify user or coordinator of timeout
- **Abort:** Stop workflow and require manual intervention

## Advanced Patterns

### Continuous Polling

Use `--listen` mode for continuous message monitoring:

```bash
# Poll indefinitely (until timeout)
hive msg sub --listen --topic notifications --timeout 1h

# Process each message as it arrives
```

**Differences from `--wait`:**
- `--wait`: Returns after ONE message
- `--listen`: Continues polling, outputs ALL messages until timeout

### Conditional Wait

Wait for specific message content:

```bash
# Wait and filter with jq
hive msg sub --wait --topic status | jq 'select(.content | contains("success"))'
```

### Multi-Topic Wait

Wait for message on any of several topics (using wildcards):

```bash
# Wait for response from any agent
hive msg sub --wait --topic "response.*"

# Wait for any build status update
hive msg sub --wait --topic "build.*.complete"
```

## Troubleshooting

### Timeout Reached Without Message

**Problem:** Wait command times out before message arrives

**Solutions:**
- Increase timeout: Use longer `--timeout` duration
- Check topic name: Verify exact topic spelling and format
- Verify sender: Ensure other agent is publishing to correct topic
- Check timing: Other agent may need more time to complete work

### Message Missed

**Problem:** Message was sent but wait didn't receive it

**Solutions:**
- Check timing: Was message sent BEFORE wait started?
- Verify topic: Ensure exact topic match (case-sensitive)
- Review message log: Check `$XDG_DATA_HOME/hive/messages/topics/<topic>.json`
- Use `--peek` first: Preview existing messages before waiting

### Wait Blocks Forever

**Problem:** Wait command never returns and never times out

**Solutions:**
- Check default timeout: Default is 30s, may need explicit `--timeout`
- Verify topic exists: Use `hive msg list` to check topics
- Test with short timeout: Use `--timeout 5s` to debug
- Check polling: Verify no errors in polling loop

### Wrong Message Received

**Problem:** Wait returns unexpected message

**Solutions:**
- Use specific topics: Avoid overly broad wildcards
- Check message history: Review with `hive msg sub --topic <topic>`
- Filter results: Use `jq` or other tools to filter messages
- Peek first: Use `--peek` to review messages before acknowledging

## Related Skills

- `/hive:inbox` - Check your inbox for messages
- `/hive:publish` - Send messages to other agents
- `/hive:session-info` - Get your session details and inbox topic

## Tips

**Choose Appropriate Timeouts:**
- Start with default 30s for typical coordination
- Use longer timeouts for known slow operations
- Use shorter timeouts for fast-fail scenarios

**Be Explicit:**
- Use specific topic names, not wildcards, when possible
- Document expected wait times in workflow docs
- Log timeout events for debugging

**Handle Failures:**
- Always handle timeout scenarios gracefully
- Provide fallback behavior when waits fail
- Alert users or coordinators when critical waits time out

**Test Wait Logic:**
- Test with short timeouts during development
- Simulate delays to verify timeout handling
- Document expected wait patterns in code comments
