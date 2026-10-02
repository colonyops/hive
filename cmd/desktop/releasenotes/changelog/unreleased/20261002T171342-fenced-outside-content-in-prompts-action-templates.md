---
kind: added
---

**Fenced outside content in prompts.** Action templates can wrap a PR body, an issue, or a webhook payload in `{{ untrustedStart }}` and `{{ untrustedEnd }}`, so the agent can tell where your instructions stop. The starter actions, the New Session draft from an item, and the webhook transform prompt now fence the item text this way.
