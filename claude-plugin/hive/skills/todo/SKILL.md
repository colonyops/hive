---
name: todo
description: This skill should be used when the agent writes files to the .hive/ context directory (plans, research docs) and wants the operator to review them, or when the agent needs to create a TODO item for the operator. Also use when the agent asks "how do I request review?", "how do I notify the operator?", or "how do I use hive todo?".
compatibility: claude, opencode
---

# TODO - Request Operator Review

Notify the operator to review your work by including frontmatter in context directory files and using `hive todo create` for explicit review requests.

## How It Works

Hive watches the `.hive/` context directory for file changes. When you write plans, research, or other documents, the operator's TUI automatically detects new files and shows them as TODO items for review. Including frontmatter with your `session_id` enables the operator to send feedback directly to your inbox.

## Frontmatter Format

When writing files to `.hive/` (plans, research, notes), include a frontmatter block:

```markdown
---
session_id: <your-session-id>
title: <descriptive title>
---

# Your Document Content
```

### Getting Your Session ID

```bash
SESSION_ID=$(hive session info --json | jq -r '.id')
```

Or use `hive session info` for the human-readable version.

### Example

```bash
SESSION_ID=$(hive session info --json | jq -r '.id')
cat > .hive/plans/my-feature-plan.md << EOF
---
session_id: $SESSION_ID
title: Feature X Implementation Plan
---

# Feature X Plan

## Overview
...
EOF
```

The operator will see "Feature X Implementation Plan" in their TODO panel and can review, comment, and send feedback to your inbox.

## Explicit TODO Items

For actionable items that need operator attention beyond file reviews:

```bash
hive todo create "Please review the API design in .hive/plans/api-design.md"
```

### Guidelines

- Use sparingly -- only for items that genuinely need human attention
- Be specific about what you need reviewed
- File changes in `.hive/` are auto-detected, so `hive todo create` is for supplementary requests
- Rate limited to prevent abuse (default: 20 per session per hour)

## What Happens Next

1. Operator sees a `[N todo]` indicator in their TUI tab bar
2. Pressing `t` opens the TODO action panel listing your item
3. Operator selects the item to open it in the Review tab
4. After reviewing, operator can send feedback to your inbox (if `session_id` was provided)
5. Check your inbox: `hive msg inbox`

## Commands

| Command | Description |
|---------|-------------|
| `hive todo create "<title>"` | Create a TODO item for the operator |
| `hive todo list` | List pending TODO items (JSON) |
| `hive todo dismiss <id>` | Dismiss a TODO item |

## Related Skills

- `/hive:inbox` - Check inbox for operator feedback
- `/hive:session-info` - Get your session ID
- `/hive:publish` - Send messages to other agents
