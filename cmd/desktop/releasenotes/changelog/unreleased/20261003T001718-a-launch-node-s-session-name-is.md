---
kind: fixed
---

A launch node's session name is capped at 60 characters, falls back to a name from the node and the item when the template renders no letters or digits, and gets a `-2` suffix when another session already has it, so `review-{{ .Payload.repo }}` no longer fails the second pull request in a repository. A session name is unique on its slug, so `Fix Bug` and `fix-bug` count as the same name.
