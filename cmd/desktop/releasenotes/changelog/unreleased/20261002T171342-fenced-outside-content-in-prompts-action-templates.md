---
kind: added
---

**Fenced outside content in prompts.** Action and node templates can wrap a PR body, an issue, or a webhook payload in `{{ untrustedStart }}` and `{{ untrustedEnd }}`, so the agent can tell where your instructions stop. `untrustedStart` takes key/value pairs for attributes on the opening tag, and `{{ untrustedNotice }}` explains the fence to the agent. The starter actions, the New Session draft from an item, and the webhook transform prompt now fence the item text this way.
