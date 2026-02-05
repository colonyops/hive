---
name: hive:publish
description: Send messages to other agents or broadcast to multiple sessions. Use when coordinating work, sending notifications, or communicating with other Claude sessions.
compatibility: claude, opencode
---

# Publish - Send Inter-Agent Messages

Send messages to other agents, broadcast to multiple sessions, or publish notifications to topic-based channels.

## When to Use

Use this skill when:
- Handing off work to another agent
- Notifying other sessions of events
- Broadcasting status updates
- Sending results or data to waiting agents
- Coordinating distributed work

**Common triggers:**
- "send message to agent X"
- "publish to topic"
- "broadcast to all agents"
- "notify other sessions"
- "tell agent Y that..."

## How It Works

Messages are published to named topics stored as JSON files at `$XDG_DATA_HOME/hive/messages/topics/`. Other agents can subscribe to these topics to receive messages.

The sender is auto-detected from your current hive session context. Each message includes:
- Sender ID (your session)
- Timestamp
- Content
- Topic(s)

## Commands

### Publish Direct Message

Send a message as a command-line argument:

```bash
hive msg pub --topic <topic> "message content"
```

**Examples:**
```bash
# Send to specific agent's inbox
hive msg pub --topic agent.abc123.inbox "Task completed, ready for review"

# Publish to custom topic
hive msg pub --topic build.status "Build #42 completed successfully"

# Send to multiple topics
hive msg pub -t agent.123.inbox -t agent.456.inbox "Important update"
```

### Publish from Stdin

Pipe content directly to publish:

```bash
echo "Status update" | hive msg pub --topic notifications
cat report.txt | hive msg pub --topic reports
```

**Use cases:**
- Publishing command output
- Sending file contents
- Piping structured data

### Publish from File

Read message content from a file:

```bash
hive msg pub --topic logs -f build.log
hive msg pub -t agent.abc.inbox --file handoff-context.md
```

**Use cases:**
- Sending large messages
- Publishing log files
- Sharing structured data or reports

### Broadcast with Wildcards

Use wildcards to publish to multiple matching topics:

```bash
# Broadcast to all agent inboxes
hive msg pub -t "agent.*.inbox" "System maintenance in 5 minutes"

# Notify all build watchers
hive msg pub -t "build.*.status" "Build server online"
```

**Wildcard patterns:**
- `*` matches any characters within a segment
- Topics are expanded at publish time
- Messages are sent to all matching topics

## Topic Patterns

### Standard Inbox Format

Agent inboxes follow the pattern: `agent.<session-id>.inbox`

```bash
# Find your session ID
hive session ls

# Send to specific agent
hive msg pub --topic agent.abc123.inbox "Message"
```

### Custom Topics

Create any topic name for specific use cases:

```bash
# Event notifications
hive msg pub --topic deploy.started "Deployment initiated"
hive msg pub --topic deploy.completed "Deployment finished"

# Coordination channels
hive msg pub --topic coordinator.requests "Need help with task X"
hive msg pub --topic build.events "Test suite running"
```

### Topic Naming Conventions

**Good topic names:**
- `agent.<id>.inbox` - Direct messages to agents
- `<domain>.<event>` - Event notifications (build.started, test.failed)
- `<feature>.<channel>` - Feature-specific channels (auth.status, cache.updates)

**Avoid:**
- Generic names (messages, data, updates)
- Special characters besides `.`, `-`, `_`
- Very long topic names

## Examples

### Direct Handoff to Another Agent

```bash
# You: Complete work and notify next agent
hive msg pub --topic agent.xyz789.inbox "Auth implementation complete. See PR #123. Tests passing."

# Agent XYZ receives in their inbox
hive msg inbox
# > [2026-02-04 10:30] agent.abc123: Auth implementation complete. See PR #123. Tests passing.
```

### Broadcast Status Update

```bash
# Notify all agents monitoring build status
hive msg pub -t "build.*.status" "Build server restarting for maintenance"

# Any agent subscribed to build.main.status, build.dev.status, etc. receives the message
```

### Multi-Topic Notification

```bash
# Send to multiple specific recipients
hive msg pub \
  -t agent.abc.inbox \
  -t agent.xyz.inbox \
  -t coordinator.updates \
  "Feature X ready for review"
```

### Send Command Output

```bash
# Publish test results
go test -v ./... | hive msg pub --topic test.results

# Share file list
ls -la | hive msg pub --topic inventory

# Send structured data
jq '.' report.json | hive msg pub --topic reports.daily
```

### File-Based Message

```bash
# Send detailed context from file
hive msg pub --topic agent.abc.inbox -f handoff-notes.md

# Publish entire log file
hive msg pub --topic logs.deployment --file deploy.log
```

## Coordinated Workflows

### Work Handoff Pattern

**Agent A (completing work):**
```bash
# Complete task
git commit -m "feat: implement feature X"

# Notify next agent with context
hive msg pub --topic agent.bob.inbox "Feature X implementation complete.
Branch: feat/feature-x
Tests: All passing
Next: Need integration testing with component Y
See issue: hive-123"
```

**Agent B (receiving work):**
```bash
# Check inbox
hive msg inbox
# > Message from agent.alice: Feature X implementation complete...

# Start work on next step
bd show hive-123
```

### Event Broadcasting Pattern

**Build Agent:**
```bash
# Start build
hive msg pub --topic build.main.status "Build started: commit abc123"

# Complete build
hive msg pub --topic build.main.status "Build completed: all tests passed"
```

**Monitoring Agent:**
```bash
# Subscribe to build events
hive msg sub build.main.status

# Receives all build status updates
```

### Acknowledgment Pattern

**Sender (waiting for confirmation):**
```bash
# Send message
hive msg pub --topic agent.bob.inbox "Task ready for you"

# Subscribe to acknowledgment topic
hive msg sub agent.bob.inbox.ack

# Wait for Bob to read the message
```

**Receiver (Bob):**
```bash
# Read message (automatically sends acknowledgment)
hive msg inbox
# Acknowledgment is automatically published to sender
```

## Advanced Usage

### Override Sender ID

Useful for testing or special coordination scenarios:

```bash
hive msg pub --topic test.channel --sender "custom-agent-id" "Test message"
```

**Use cases:**
- Testing message workflows
- Proxy messages from external systems
- Debugging topic communication

### Multiple Topic Publishing

Send same message to multiple distinct topics:

```bash
hive msg pub \
  -t notifications \
  -t agent.alice.inbox \
  -t agent.bob.inbox \
  -t coordinator.events \
  "Critical: Database migration starting"
```

## Troubleshooting

### Message Not Received

**Problem:** Published message doesn't appear in recipient's inbox

**Solutions:**
- Verify topic name: Check recipient's session ID with `hive session ls`
- Correct format: Use `agent.<session-id>.inbox` for direct messages
- Check message file: Look in `$XDG_DATA_HOME/hive/messages/topics/<topic>.json`

### Wildcard Not Expanding

**Problem:** Wildcard pattern doesn't match expected topics

**Solutions:**
- Check existing topics: `hive msg list`
- Verify pattern syntax: Wildcards only work within segments
- Create topics first: Topics must exist to be matched

### Sender ID Incorrect

**Problem:** Messages show wrong sender

**Solutions:**
- Verify session context: Run from correct hive session directory
- Check session: `hive session ls` to see active sessions
- Override if needed: Use `--sender` flag explicitly

## Related Skills

- `/hive:inbox` - Check your inbox for messages
- `/hive:wait` - Wait for messages with timeout
- `/hive:session-info` - Get your session details and inbox topic

## Tips

**Be Clear:**
- Include context in messages (what, why, next steps)
- Reference issues, PRs, or branches when relevant
- Use structured format for complex messages

**Be Efficient:**
- Use stdin for command output
- Use files for large or structured data
- Use wildcards for broadcasts

**Be Coordinated:**
- Follow up on handoffs by checking acknowledgments
- Subscribe to relevant topics for updates
- Document coordination patterns in team workflows
