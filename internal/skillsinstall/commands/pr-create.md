---
description: Create a PR with review
argument-hint: <tags>
---

## Task

Create a pull request for the changes. If specified, ensure #$ARGUMENTS tags are added to the PR.

## Workflow

Since the repository state is unknown, you may need to:

1. Create a new branch
2. Commit code changes
3. Push the branch
4. Create the PR using the GitHub CLI

## PR Template

If `PULL_REQUEST_TEMPLATE.md` exists in the repository, use it as the basis for your PR description.

## Writing the Description

**Be brief.** Reviewers will read the code—don't explain it line by line.

- 1-3 sentences for the summary, max
- Focus on _why_ the change was made, not _what_ changed (the diff shows that)
- Only mention non-obvious decisions or gotchas
- Small changes = small descriptions. A typo fix needs one line, not a paragraph.

**Do NOT:**
- List every file or function modified
- Explain obvious code changes
- Add sections for the sake of completeness
- Use bullet points when a sentence will do

## Example

If there is no PULL_REQUEST_TEMPLATE.md, use this format:

```
## Summary

Added rate limiting to the API to prevent abuse. Uses a token bucket algorithm with configurable limits per endpoint.
```

For trivial changes, a single line title is sufficient:

```
Fix typo in error message
```
