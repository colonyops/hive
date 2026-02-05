---
name: hive:session-info
description: Get your current session ID, inbox topic, and session information. Use when you need to know your agent identity for messaging coordination.
compatibility: claude, opencode
---

# Session Info - Get Your Session Details

Display information about your current hive session, including session ID, inbox topic, and state.

## When to Use

Use this skill when:
- You need to know your session ID
- Another agent asks for your inbox topic
- Debugging messaging issues
- Setting up inter-agent coordination
- Verifying you're in the correct session

**Common triggers:**
- "what's my session ID?"
- "show my inbox topic"
- "get session info"
- "what session am I in?"
- "my agent ID"

## How It Works

The `hive session info` command detects your current session based on your working directory and displays relevant information including:

- **Session ID**: Unique identifier for this agent session
- **Name**: Human-readable session name
- **Repository**: Git repository name
- **Inbox**: Your inbox topic for receiving messages
- **Path**: Full path to session directory
- **State**: Current session state (active, recycled, corrupted)

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

For programmatic access or LLM parsing:

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

## Session Detection

The command automatically detects your session from the current working directory. Sessions are identified by:

1. **Working directory path**: Must be within a hive session directory
2. **Session directory pattern**: `$XDG_DATA_HOME/hive/repos/<repo>-<session-id>`
3. **Session file**: `.hive-session` file in the directory

If you're not in a hive session directory, the command will fail with an error.

## Session States

### Active
Normal, working session. The session is ready for use.

**Characteristics:**
- Session directory exists
- All required files present
- Ready for commands and messaging

### Recycled
Session has been recycled (stopped and cleared).

**Characteristics:**
- Session may still exist on disk
- Not actively running
- Can be restarted or removed

### Corrupted
Session directory or files are damaged or incomplete.

**Characteristics:**
- Missing required files
- Inconsistent state
- May need manual cleanup or recreation

## Inbox Topic Format

Your inbox topic follows the pattern: `agent.<session-id>.inbox`

**Example:**
- Session ID: `26kj0c`
- Inbox: `agent.26kj0c.inbox`

Other agents use this topic to send you direct messages:

```bash
# Another agent sends to you
hive msg pub --topic agent.26kj0c.inbox "Message for you"

# You receive it
hive msg inbox
```

## Use Cases

### Share Your Inbox with Another Agent

```bash
# Get your inbox topic
hive session info --json | jq -r '.inbox'
# Output: agent.26kj0c.inbox

# Tell another agent
"Send messages to agent.26kj0c.inbox"
```

### Verify Session Before Messaging

```bash
# Check you're in the right session
hive session info

# Then check inbox
hive msg inbox
```

### Debug Messaging Issues

```bash
# Get full session details
hive session info

# Verify inbox topic matches expectations
# Check state is "active"
```

### Script Session Information

```bash
# Extract specific fields
SESSION_ID=$(hive session info --json | jq -r '.id')
INBOX=$(hive session info --json | jq -r '.inbox')

echo "My session: $SESSION_ID"
echo "Send messages to: $INBOX"
```

## Output Fields

### id
Short unique identifier for this session (e.g., `26kj0c`).

Used in:
- Inbox topic construction
- Session directory naming
- Message sender identification

### name
Human-readable name describing the session purpose (e.g., `claude-plugin`, `fix-auth-bug`).

Helps identify what work this session is doing.

### repository
Git repository name this session is working on (e.g., `hive`, `myproject`).

### inbox
Full inbox topic for receiving messages: `agent.<session-id>.inbox`.

Other agents publish to this topic to send you messages.

### path
Absolute path to the session directory on disk.

Useful for:
- Verifying session location
- Debugging path issues
- Scripting file operations

### state
Current session state: `active`, `recycled`, or `corrupted`.

Indicates session health and readiness.

## Examples

### Basic Session Info

```bash
$ hive session info
Session ID:  abc123
Name:        implement-auth
Repository:  backend
Inbox:       agent.abc123.inbox
Path:        /home/user/.local/share/hive/repos/backend-abc123
State:       active
```

### JSON Output for Parsing

```bash
$ hive session info --json
{
  "id": "abc123",
  "name": "implement-auth",
  "repository": "backend",
  "inbox": "agent.abc123.inbox",
  "path": "/home/user/.local/share/hive/repos/backend-abc123",
  "state": "active"
}
```

### Extract Inbox Topic

```bash
$ hive session info --json | jq -r '.inbox'
agent.abc123.inbox
```

### Share Your Info with Another Agent

```bash
# Agent A (you)
$ hive session info --json
{
  "id": "abc123",
  "inbox": "agent.abc123.inbox",
  ...
}

# Send to Agent B
$ hive msg pub --topic agent.xyz789.inbox "My inbox is agent.abc123.inbox. Send results there."

# Agent B receives and replies
$ hive msg pub --topic agent.abc123.inbox "Got it, will send results to you"
```

## Troubleshooting

### Not in a Hive Session

**Problem:** Command fails with "not in a hive session" error

**Solutions:**
- Verify working directory: `pwd`
- Check if in hive session: Look for `.hive-session` file
- List sessions: `hive session ls` (if available)
- Change to correct directory

### Session State is "corrupted"

**Problem:** Session info shows state as "corrupted"

**Solutions:**
- Check session directory exists: Verify path from output
- Look for missing files: `.hive-session`, tmux socket, etc.
- Consider recreating session
- Check disk space and permissions

### Inbox Topic Not Working

**Problem:** Messages sent to inbox don't appear

**Solutions:**
- Verify exact inbox topic: Copy from `hive session info` output
- Check for typos: Topics are case-sensitive
- Confirm messages exist: `hive msg sub --topic <your-inbox>`
- Test with known-good message: Send to yourself

## Related Skills

- `/hive:inbox` - Check your inbox for messages
- `/hive:publish` - Send messages to other agents' inboxes
- `/hive:wait` - Wait for messages on your inbox

## Tips

**Be Explicit:**
- Share exact inbox topic (don't paraphrase)
- Use JSON output for programmatic access
- Include session ID in coordination messages

**Verify First:**
- Check session info before setting up coordination
- Confirm state is "active" before messaging
- Validate inbox topic format matches pattern

**Script Safely:**
- Use JSON output for reliable parsing
- Handle missing fields gracefully
- Check exit codes for errors
