---
icon: lucide/repeat
---

# Ralph Loop

Work through a beads backlog autonomously by running a fresh Claude process per task,
iterating until quality gates pass before moving on.

!!! note "Requirements"
    Requires `claude` CLI in PATH with a valid API key. Tasks are read from the
    `.beads/` directory in the selected session. The loop runs in a dedicated tmux
    window so the TUI stays responsive.

## How It Works

1. Pick the next open beads task (`bd ready`)
2. Write task details to `.hive/ralph-task.md`
3. Run `claude --dangerously-skip-permissions -p` with the task as the prompt
4. After Claude exits, run your configured check command
5. If checks pass — close the task and continue
6. If checks fail — retry up to `MAX_RETRIES` times, then release the task and move on
7. Repeat until no tasks remain

## Config

Add to your `~/.config/hive/config.yaml`:

```yaml
usercommands:
  RalphLoop:
    help: "Run ralph loop on beads backlog"
    form:
      - variable: check_cmd
        type: text
        label: "Check command"
        placeholder: "mise run check"
      - variable: max_retries
        type: text
        label: "Max retries per task"
        placeholder: "3"
    sh: |
      tmux new-window -t {{ .Name | shq }} -n "ralph" -- \
        bash -c 'bash ~/.config/hive/scripts/ralph-loop.sh \
          {{ .Path | shq }} \
          {{ .Form.check_cmd | shq }} \
          {{ .Form.max_retries | shq }}; exec bash'
```

## Script

Save to `~/.config/hive/scripts/ralph-loop.sh` and make it executable:

```bash
#!/usr/bin/env bash
set -euo pipefail

SESSION_PATH="${1:?Usage: ralph-loop.sh <session-path> [check-cmd] [max-retries]}"
CHECK_CMD="${2:-mise run check}"
MAX_RETRIES="${3:-3}"

cd "$SESSION_PATH"

while true; do
    TASK=$(bd ready --json 2>/dev/null | jq -e '.[0]') || {
        echo "No tasks remaining. Done."
        break
    }

    ID=$(jq -r '.id'                  <<< "$TASK")
    TITLE=$(jq -r '.title'            <<< "$TASK")
    DESC=$(jq -r '.description // ""' <<< "$TASK")

    echo ""
    echo "==> $ID: $TITLE"
    bd update "$ID" --status in_progress

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
        bd close "$ID"
        echo "   Closed: $ID"
    else
        echo "   Gave up after $MAX_RETRIES attempts: $ID — releasing"
        bd update "$ID" --status open
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

A form prompts for the check command and max retries, then a `ralph` window opens
in the current tmux session. Switch to it to watch progress. The TUI remains usable
while the loop runs.

**Leave check command empty** to skip quality gate verification and trust Claude's
own judgment on when a task is complete.

## Customization

**Task prompt** — edit the heredoc in the script to give Claude additional context,
coding standards, or constraints that apply to all tasks in the loop.

**Task filter** — `bd ready` picks tasks by priority. Replace the task selection
line with a more specific `bd` invocation to filter by label or type.
