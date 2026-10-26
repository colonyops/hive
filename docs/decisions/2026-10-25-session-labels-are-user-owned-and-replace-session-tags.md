# Session labels are user-owned and replace session tags

- **Status:** accepted
- **Date:** 2026-10-25

## Context

Users need to label sessions by hand and from workflows, and later work will
filter sessions and build views on those labels. An orchestration agent, for
example, should find every session labelled `auto`.

Sessions already had a `tags` list. It was set only at creation: by
`hive new --tags`, by source templates, and by Hive Desktop, which copied each
origin inbox item's external id into it. No code read a tag value to decide
anything; the desktop's item-to-session association is the `item_session`
table (ADR a-session-can-link-to-many-inbox-items).

## Decision

Tags become labels, one list per session, editable after creation.

- **One concept, one name.** The `hive.db` column, the domain field, CLI
  flags, JSON output, batch input, and the sources `templates.labels` key all
  say `labels`. The old spellings are removed, not aliased.
- **Labels belong to the user.** Hive Desktop no longer copies item ids into
  them. A program that needs to remember an association keeps its own record.
- **The rules live in `internal/domain/session`.** Labels are trimmed,
  compared without case (the first spelling is kept), at most
  `MaxLabelLength` characters, and never contain a comma, which editors and
  flags use as a separator.
- **Automation edits, users replace.** `EditSessionLabels` adds and then
  removes, so two writers that each add a label keep both.
  `SetSessionLabels` replaces the list and is for an editor that shows the
  whole list.

## Consequences

- A label is free text with no registry, colour, or description. A registry
  can come later without changing what a session stores.
- A label change from the CLI reaches Hive Desktop through the sessions probe,
  whose signature includes labels.
- An older hive binary cannot read sessions from a `hive.db` after this
  migration renames the column.
