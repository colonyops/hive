# Palette rows name objects and rendering draws the path

- **Status:** accepted
- **Date:** 2026-08-29

## Context

Palette row titles in `useAppPaletteRows` had three shapes: a catalog verb
("Go to Inbox"), a verb glued onto a noun ("Attach session: ..."), and a path
built into the title ("Frontend Triage › Notifications inbox") that repeated
the group header. `CommandPalette.vue` already draws a row's container from
`cmd.group`, as the group header on an empty query and as a per-row
`groupPrefix ›` once a query or Recent reorders rows out of their groups. A
hand-built title path showed twice.

## Decision

**A row's title names the thing; its group names what it lives inside.**

- A **command row** seeded from `bindableCommands` keeps the catalog's verb
  title, because `keymapRows` shows the same `command.title` in
  Settings > Keyboard, where it must read correctly alone.
- A **dynamic object row** (a feed, Trash, a chat, a session, a window, a
  profile, a settings section) sets `title` to the object's own name and
  `group` to its real container: the active profile's name for feeds, the
  workspace's display name for chats (`workspaceNameByDir`, falling back to the
  directory key), the terminal group for session rows, and the attached
  session's name for window rows. No verb prefix and no `A › B` in the title.

Nesting is drawn once, by the palette's rendering. The verb a title used to
carry moves to `keywords`, so "attach repo" still ranks the row. `hint` is only
a shortcut combo or a numbered jump chord. An object row also carries `kind`, a
muted type word ("window", "feed") at the right edge, because All interleaves
bare names with verb rows and a name alone does not say what Enter does. `kind`
names the type, never the container.

Group labels are dynamic, so `order` keeps section order: chats at `-2`, the
active profile's feeds and Trash at `-1`.

## Consequences

- A new object row sets `title` to its leaf name and `group` to its container,
  puts any verb in `keywords`, and never encodes the container in the title.
