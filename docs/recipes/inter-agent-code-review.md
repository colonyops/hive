---
icon: lucide/git-pull-request
---

# Inter-Agent Code Review

Orchestrate code reviews by spinning up a dedicated reviewer agent that receives context via inbox messaging, reviews your branch, and sends feedback back to you.

!!! note "Requirements"
    This recipe requires the `agent-send` bundled script (available via `{{ agentSend }}`) and the `/hive-msg` skill for inbox messaging. Both are available out of the box with the tmux plugin enabled.

## Config

Add these user commands to your `~/.config/hive/config.yaml`:

```yaml
usercommands:
  ReviewRequest:
    sh: "~/.config/hive/scripts/request-review.sh {{ .Name | shq }} {{ agentSend | shq }} {{ agentWindow | shq }}"
    help: "Request code review of current branch"
    silent: false

  CheckInbox:
    sh: "hive msg sub -t agent.{{ .ID }}.inbox --new"
    help: "Check inbox for new messages"
    silent: false
```

## Scripts

### request-review.sh

Creates a new hive session with a unique ID, sends review context to its inbox, and instructs your current session to send detailed review instructions via the `/hive-msg` skill.

Save to `~/.config/hive/scripts/request-review.sh`:

```bash
#!/bin/bash
# Usage: request-review.sh <session-name> <agent-send-path> <agent-window>
# Requests a code review of the current branch

CURRENT_SESSION_NAME="${1:-}"
AGENT_SEND="${2:-}"
AGENT_WINDOW="${3:-claude}"
CURRENT_REPO=$(git remote get-url origin 2>/dev/null)
CURRENT_BRANCH=$(git branch --show-current)

if [ -z "$CURRENT_BRANCH" ]; then
    echo "Error: Not on a git branch"
    exit 1
fi

if [ -z "$CURRENT_SESSION_NAME" ]; then
    echo "Error: Session name required"
    exit 1
fi

if [ -z "$AGENT_SEND" ]; then
    echo "Error: agent-send path required"
    exit 1
fi

# Sanitize branch name for use in session name (replace / with -)
SAFE_BRANCH=$(echo "$CURRENT_BRANCH" | tr '/' '-')

# Generate unique session ID for reviewer
NEW_SESSION=$(hive msg topic --prefix "")

# Create new review session
BATCH_JSON=$(cat <<EOF
{
  "sessions": [
    {
      "session_id": "$NEW_SESSION",
      "name": "review-$SAFE_BRANCH-$NEW_SESSION",
      "origin": "$CURRENT_REPO",
      "prompt": "You are a code reviewer. Check your inbox (agent.$NEW_SESSION.inbox) for review instructions using: hive msg sub -t agent.$NEW_SESSION.inbox --new. Wait until you receive a message. If no messages are available use hive msg sub -t agent.$NEW_SESSION.inbox --wait to wait for a message."
    }
  ]
}
EOF
)

echo "$BATCH_JSON" | hive batch

echo "✓ Created reviewer session: $NEW_SESSION (review-$SAFE_BRANCH-$NEW_SESSION)"
echo "✓ Reviewer inbox: agent.$NEW_SESSION.inbox"
echo ""

# Instruct current session to send review context via /hive-msg
if [ -x "$AGENT_SEND" ]; then
    "$AGENT_SEND" "$CURRENT_SESSION_NAME:$AGENT_WINDOW" "/hive-msg a reviewer is waiting for a message from you at agent.$NEW_SESSION.inbox. Please send them context on the work you are doing and how they can access your branch and review the code. Wait up to 1 hour for a response"
    echo "✓ Sent instructions to current session"
else
    echo "Note: agent-send not found at: $AGENT_SEND"
    echo "  /hive-msg a reviewer is waiting at agent.$NEW_SESSION.inbox"
fi
```

Make it executable:
```bash
chmod +x ~/.config/hive/scripts/request-review.sh
```

## Usage

### 1. Request a Review

In the hive TUI, select your active session and press `:`:
```
:ReviewRequest
```

This will:
- Create a new reviewer session with a unique ID
- Set up the reviewer to listen on their inbox
- Trigger your current session to send review context via `/hive-msg`

### 2. Send Review Context

Your session will automatically run `/hive-msg` with instructions to send:
- Branch name and repository info
- Recent commits and changes
- How to checkout and review the code
- Your inbox address for feedback

### 3. Wait for Feedback

Check for the reviewer's response:
```
:CheckInbox
```

Or use the messaging skill:
```
/hive-msg
```

The reviewer will analyze your code and send feedback to your inbox.

## How It Works

1. **Session Creation**: `hive batch` creates a new session with a unique inbox
2. **Inbox Setup**: The reviewer session is prompted to listen on `agent.{session-id}.inbox`
3. **Context Sharing**: Your session sends review instructions via `hive msg pub`
4. **Response Loop**: You check your inbox for the reviewer's feedback
5. **Async Communication**: Both agents work independently, coordinated via messaging

## Requirements

- `agent-send` - Bundled script (via `{{ agentSend }}` template variable) to send text to tmux windows
- `/hive-msg` skill - Handles inbox messaging and context generation

## Customization

**Change review prompt**: Edit the `prompt` field in the BATCH_JSON to customize reviewer behavior.

**Add review checklist**: Modify the `/hive-msg` instruction to include specific review criteria.

**Auto-respond**: Add a follow-up command to automatically respond to reviewer feedback.
