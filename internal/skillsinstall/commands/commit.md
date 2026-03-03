---
allowed-tools: Bash(git add:*), Bash(git status:*), Bash(git commit -m:*)
description: Stage files and create commits with approval
---

Review the current changes and generate a concise commit message.
Return ONLY the commit message text with no additional formatting or explanation.

The first line should clearly summarize the change.
Do not include backticks, quotes, or any other formatting.

Example:
Instead of something like:

> Update user authentication flow to handle new validation rules
>
> This extends validation checks from login-only to both login and registration, preparing for broader testing across supported workflows.

Write:
Add validation rules to registration flow

Keep commit messages proportional to the size of the change:
- Small one-line changes → one-line commit message.
- Larger features → a few lines describing what was added and why.

Once the commit message is ready:
1. Show me the list of staged files and the proposed commit message.
2. Wait for my approval.
3. If I approve, run `git commit -m "<message>"` with the approved message and staged files.
