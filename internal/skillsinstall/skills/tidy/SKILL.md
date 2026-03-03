---
name: tidy
description: Clean up workspace and complete session - commit, push, sync issues, and verify
argument-hint:
---

# Tidy - Session Completion Checklist

Complete all work and clean up the workspace. Work is NOT complete until `git push` succeeds.

## Usage

```
/tidy
```

Run this at the end of any work session to ensure all changes are committed, pushed, and issues are updated.

## Mandatory Workflow

Execute ALL steps in order. Do not skip steps.

### Step 1: Assess Current State

Run these commands to understand the workspace state:

```bash
git status
git stash list
hive hc list --status in_progress
```

Identify:
- Uncommitted changes
- Untracked files that should be committed or deleted
- Stashed changes that need resolution
- In-progress issues that need updates

### Step 2: Handle Uncommitted Changes

If there are changes:

1. **Review what changed**: `git diff`
2. **Stage relevant files**: `git add <specific-files>` (avoid `git add -A`)
3. **Commit with clear message**: Follow repo's commit style
4. **Delete temporary files**: Remove any scratch files, debug logs, etc.

If changes shouldn't be committed:
- Stash if needed later: `git stash push -m "description"`
- Or discard: `git checkout -- <file>` (only if truly unwanted)

### Step 3: Run Quality Gates (if code changed)

Only run if actual code was modified (not just docs or config):

```bash
# Language-specific - run what applies
go test ./...
go vet ./...
npm test
npm run lint
cargo test
cargo clippy
```

Fix any failures before proceeding. Do not push broken code.

### Step 4: Update Issue Status

```bash
# Check current issues
hive hc list

# Close completed work
hive hc update <issue-id> --status done

# Update in-progress items
hive hc update <issue-id> --status open
```

### Step 5: File Issues for Remaining Work

If there's follow-up work needed:

```bash
hive hc create --title "Follow-up: description"
```

Include:
- What needs to be done
- Why it wasn't done now
- Any relevant context or file references

### Step 6: Sync and Push

This is **MANDATORY**. Work is not complete until pushed.

```bash
# Pull any remote changes
git pull --rebase

# Push all changes
git push

# Verify push succeeded
git status
```

The output MUST show "Your branch is up to date with 'origin/...'"

If push fails:
1. Resolve conflicts
2. Re-run quality gates
3. Push again
4. Repeat until successful

### Step 7: Clean Up

```bash
# Clear resolved stashes
git stash list
git stash drop <stash@{n}>  # For each resolved stash

# Prune deleted remote branches
git fetch --prune

# Clean up merged local branches (optional)
git branch --merged | grep -v "main\|master\|\*" | xargs git branch -d
```

### Step 8: Final Verification

Run final checks:

```bash
git status                    # Should show clean working tree
git log -1 --oneline         # Verify last commit is yours
git log origin/main..HEAD    # Show commits ahead of main (if on branch)
hive hc list --status open   # Show remaining open issues
```

## Output Format

After completing all steps, report:

```markdown
## Session Tidy Complete

### Commits
- [commit-sha] [commit-message]

### Issues Updated
- Closed: #[id] [title]
- Created: #[id] [title]

### Status
- Working tree: Clean
- Remote sync: Up to date
- Open issues: [N] remaining

### Notes
[Any follow-up context for next session]
```

## Critical Rules

1. **NEVER stop before pushing** - Local-only commits are lost work
2. **NEVER say "ready to push when you are"** - YOU must push
3. **NEVER skip quality gates** - Don't push broken code
4. **NEVER commit sensitive files** - Check for .env, credentials, keys
5. **ALWAYS verify push succeeded** - Check `git status` output

## Error Recovery

### Push rejected (non-fast-forward)

```bash
git pull --rebase
# Resolve any conflicts
git push
```

### Tests failing

```bash
# Fix the issue first
# Re-run tests
# Only then commit and push
```

### Merge conflicts

```bash
# Resolve conflicts in each file
git add <resolved-files>
git rebase --continue
git push
```

### Stash conflicts

```bash
# Apply stash to see conflicts
git stash pop
# Resolve conflicts
# Either commit or re-stash with new context
```

## When NOT to Use /tidy

- In the middle of active work (finish the task first)
- When you need to context-switch urgently (use `git stash` instead)
- For quick questions or research sessions (no code changed)

## Guidelines

1. **Complete the loop** - Every session should end with a clean, pushed state
2. **Document what's left** - File issues for any incomplete work
3. **Update issue status** - Keep the backlog accurate
4. **Verify everything** - Don't assume commands succeeded
5. **Hand off cleanly** - Next session (or person) should have full context
