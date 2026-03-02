---
icon: lucide/repeat
---

# Ralph Loop

Work through a task backlog autonomously by running a fresh Claude process per task,
iterating until quality gates pass before moving on.

!!! note "Requirements"
    Requires `claude` CLI in PATH with a valid API key. Tasks are managed via
    `hive hc` and scoped to the repository. The loop runs in a dedicated tmux
    window so the TUI stays responsive.

## How It Works

1. Claim the next open task from an epic (`hive hc next --assign`)
2. Write task details to `.hive/ralph-task.md`
3. Run `claude --dangerously-skip-permissions -p` with the task as the prompt
4. After Claude exits, run your configured check command
5. If checks pass — mark the task done and continue
6. If checks fail — retry up to `MAX_RETRIES` times, then release the task and move on
7. Repeat until no tasks remain

## Setup

### 1. Create an epic with tasks

```bash
echo '{
  "title": "Sprint backlog",
  "type": "epic",
  "children": [
    {"title": "Add input validation to API", "type": "task"},
    {"title": "Fix pagination bug", "type": "task"},
    {"title": "Add rate limiting middleware", "type": "task"}
  ]
}' | hive hc create
```

Note the epic ID from the output (e.g., `hc-a1b2c3d4`).

### 2. Add the user command

Add to your `~/.config/hive/config.yaml`:

```yaml
usercommands:
  RalphLoop:
    help: "Run ralph loop on hc epic backlog"
    form:
      - variable: epic_id
        type: text
        label: "Epic ID"
        placeholder: "hc-a1b2c3d4"
      - variable: check_cmd
        type: text
        label: "Check command"
        placeholder: "mise run check"
      - variable: max_retries
        type: text
        label: "Max retries per task"
        placeholder: "3"
    windows:
      - name: ralph
        focus: true
        command: >-
          bash ~/.config/hive/scripts/ralph-loop.sh
          {{ .Path | shq }}
          {{ .Form.epic_id | shq }}
          {{ .Form.check_cmd | shq }}
          {{ .Form.max_retries | shq }}
```

The `windows` field opens a tmux window named `ralph` in the current session.
See [User Commands — Multi-agent Workflows](../configuration/commands.md#multi-agent-workflows) for details on the `windows` field.

### 3. Save the script

Save to `~/.config/hive/scripts/ralph-loop.sh` and make it executable:

```bash
#!/usr/bin/env bash
set -euo pipefail

SESSION_PATH="${1:?Usage: ralph-loop.sh <session-path> <epic-id> [check-cmd] [max-retries]}"
EPIC_ID="${2:?Epic ID required}"
CHECK_CMD="${3:-mise run check}"
MAX_RETRIES="${4:-3}"

cd "$SESSION_PATH"

while true; do
    TASK=$(hive hc next "$EPIC_ID" --assign 2>/dev/null) || {
        echo "No tasks remaining. Done."
        break
    }

    ID=$(jq -r '.id'          <<< "$TASK")
    TITLE=$(jq -r '.title'    <<< "$TASK")
    DESC=$(jq -r '.desc // ""' <<< "$TASK")

    echo ""
    echo "==> $ID: $TITLE"

    cat > .hive/ralph-task.md << EOF
# $TITLE

$DESC

## Definition of Done
All quality gates must pass before you finish: \`$CHECK_CMD\`
Commit and push your changes on a feature branch when complete.
EOF

    ATTEMPT=0
    SUCCESS=false

    while [ "$ATTEMPT" -lt "$MAX_RETRIES" ]; do
        ATTEMPT=$((ATTEMPT + 1))
        echo "   Attempt $ATTEMPT/$MAX_RETRIES"

        claude --dangerously-skip-permissions \
            -p "$(cat .hive/ralph-task.md)"

        if [ -z "$CHECK_CMD" ] || eval "$CHECK_CMD"; then
            SUCCESS=true
            break
        fi

        echo "   Check failed, retrying..."
    done

    if $SUCCESS; then
        hive hc update "$ID" --status done
        echo "   Done: $ID"
    else
        echo "   Gave up after $MAX_RETRIES attempts: $ID — releasing"
        hive hc update "$ID" --status open --unassign
    fi
done

echo ""
echo "Ralph loop complete."
```

```bash
chmod +x ~/.config/hive/scripts/ralph-loop.sh
```

## Usage

Select a session in the TUI and open the command palette:

```
:RalphLoop
```

A form prompts for the epic ID, check command, and max retries, then a `ralph` window opens
in the current tmux session. Switch to it to watch progress. The TUI remains usable
while the loop runs.

**Leave check command empty** to skip quality gate verification and trust Claude's
own judgment on when a task is complete.

## Customization

**Task prompt** — edit the heredoc in the script to give Claude additional context,
coding standards, or constraints that apply to all tasks in the loop.

**Task scope** — each run targets a single epic. Create separate epics to organize
work by feature or priority, then run the loop against each.
