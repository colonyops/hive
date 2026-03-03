---
allowed-tools: Bash(gh pr:*), Bash(gh api:*)
---

Fetch comments in pr #$ARGUMENTS and review them and their contents. Make a plan on how to fix these and present them for review.

# GitHub CLI: Extracting PR Comments

Quick reference for extracting PR comments using GitHub CLI with `jq` for parsing.

## Getting Inline Code Review Comments

```bash
# Get all inline code review comments for a specific PR
gh api repos/:owner/:repo/pulls/<PR-NUMBER>/comments --paginate

# For a specific repository
gh api repos/OWNER/REPO/pulls/<PR-NUMBER>/comments --paginate
```

## Formatting Comments with jq

```bash
# Format review comments with file path, line number, and content
gh api repos/:owner/:repo/pulls/<PR-NUMBER>/comments | jq '.[] | {file: .path, line: .line, comment: .body}'

# Include author information
gh api repos/:owner/:repo/pulls/<PR-NUMBER>/comments | jq '.[] | {file: .path, line: .line, comment: .body, author: .user.login}'

# Filter for comments by a specific user
gh api repos/:owner/:repo/pulls/<PR-NUMBER>/comments | jq '.[] | select(.user.login == "username") | {file: .path, line: .line, comment: .body}'
```

## Examples

```bash
# Example for repository "hay-kot/recipinned" PR #941
gh api repos/hay-kot/recipinned/pulls/941/comments | jq '.[] | {file: .path, line: .line, comment: .body}'

# Get just comment text for all inline comments
gh api repos/hay-kot/recipinned/pulls/941/comments | jq '.[].body'

# Get file path and comment in compact format
gh api repos/hay-kot/recipinned/pulls/941/comments | jq -c '.[] | {file: .path, comment: .body}'
```

Note: The `jq` tool is available for parsing JSON responses from GitHub CLI. These commands specifically get inline comments on code, not regular PR comments.
