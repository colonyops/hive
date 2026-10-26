# Session tags are user-owned and editable

- **Status:** accepted
- **Date:** 2026-10-25

## Context

Users need to tag sessions by hand and from workflows, and later work will
filter sessions and build views on those tags. An orchestration agent, for
example, should find every session tagged `auto`.

Sessions already had a `tags` list, set only at creation: by
`hive new --tags`, by source templates, and by Hive Desktop, which copied each
origin inbox item's external id into it. No code read a tag value to decide
anything; the desktop's item-to-session association is the `item_session`
table (ADR a-session-can-link-to-many-inbox-items).

## Decision

The existing `tags` list becomes editable after creation and belongs to the
user. The field keeps its name everywhere.

- **Tags belong to the user.** Hive Desktop no longer copies item ids into
  them. A program that needs to remember an association keeps its own record.
- **The rules live in `internal/domain/session`.** Tags are trimmed,
  compared without case (the first spelling is kept), at most
  `MaxTagLength` characters, and never contain a comma, which editors and
  flags use as a separator.
- **Automation edits, users replace.** `EditSessionTags` adds and then
  removes, so two writers that each add a tag keep both. `SetSessionTags`
  replaces the list and is for an editor that shows the whole list.

## Consequences

- A tag is free text with no registry, colour, or description. A registry
  can come later without changing what a session stores.
- A tag change from the CLI reaches Hive Desktop through the sessions probe,
  whose signature includes tags.
- Tags written before this change, including desktop item ids, stay on their
  sessions until a user removes them.
