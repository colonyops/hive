---
kind: fixed
---

**A package added to `skills.yml` is picked up on save.** A workspace that enabled a package before the package existed kept reporting it as not defined until some other workspace file changed. Saving `skills.yml` now reloads the workspaces like saving `mcps.yaml` or an `agent-workspace.yaml` does.
