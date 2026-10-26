---
kind: changed
---

**Session tags are now labels.** Use `--label` (`-l`) instead of `--tags` on `hive new`, `hive session create`, and `hive ls`, `labels` instead of `tags` in `--json` output and `hive batch` input, and `templates.labels` instead of `templates.tags` in a source's config. The old spellings no longer work. Existing tags carry over as labels.
