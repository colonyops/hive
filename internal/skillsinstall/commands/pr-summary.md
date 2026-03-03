---
allowed-tools: Bash(git status:*), Bash(git log:*), Bash(git diff:*)
description: Generate a concise PR description by analyzing branch changes against main
---

Analyze the git changes and create a PR description following this format:

## Purpose

Write 1-2 sentences describing what problem this solves or what feature it adds. Focus on the "why" not the "what".

Use bullets only if there are multiple distinct changes:

- Feature/fix 1
- Feature/fix 2

## Implementation

Include this section only for complex changes that need explanation. Describe:

- Key architectural decisions
- Non-obvious approaches taken
- Important files/modules modified

Use mermaid diagrams sparingly, only when they clarify complex flows.

Example:
Instead of something like:

> Update user authentication flow to handle new validation rules
>
> This extends validation checks from login-only to both login and registration, preparing for broader testing across supported workflows.

Write:
Add validation rules to registration flow

Keep PR summaries proportional to the size of the change:

- Small one-line changes → one-line PR summary message.
- Larger features → a few lines describing what was added and why.
