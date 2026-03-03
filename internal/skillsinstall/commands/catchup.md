---
allowed-tools: Bash(git diff:*), Bash(git log:*), Bash(git status:*), Bash(git branch:*), Read, Grep, Glob
description: Show changes compared to base branch (defaults to main) (project, gitignored)
argument-hint: [any additional context, branch name, or git options]
---

Review what has changed compared to a base branch and provide a comprehensive summary.

## Arguments

User may provide any arguments: $ARGUMENTS

These could include:
- Base branch name (e.g., `develop`, `master`, `production`) - defaults to "main" if not specified
- Git diff options (e.g., `--stat`, `--name-only`)
- File paths (e.g., `src/components/`)
- Any other context or instructions they want you to consider

Be flexible and intelligent in interpreting what the user wants.

## Instructions

1. First, run `git status` to understand the current state
2. Intelligently parse $ARGUMENTS and determine:
   - What base branch to compare against (default to "main" if unclear)
   - Any additional git diff options or paths to include
   - Any other context the user provided
3. Run appropriate git commands to show the changes (typically `git diff <base-branch>...HEAD`)
4. If there are no changes, check `git log <base-branch>..HEAD` to see commits
5. Analyze the changes and provide a clear summary including:
   - Which files were modified, added, or deleted
   - Key changes in each file (what was added, removed, or modified)
   - Overall purpose and impact of these changes
   - Any patterns or themes across the changes

## Important Notes

- This is a **read-only** command - do NOT modify any files or create commits
- Be flexible with arguments - interpret user intent intelligently
- Focus on understanding and explaining the changes, not implementing new features
