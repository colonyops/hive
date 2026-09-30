# Palette rows name objects and rendering draws the path

- **Status:** accepted
- **Date:** 2026-08-29

## Context

`useAppPaletteRows` had grown three different text shapes for a row's title.
Command rows carried the catalog's own verb ("Go to Inbox", "Refresh feeds").
Some object rows copied that verb pattern onto a noun ("Attach session: ...",
"Switch to profile: ...", "Go to window: ..."). Others hand-built a path into
the title itself ("Frontend Triage › Notifications inbox"), duplicating
whatever the group header already said. Container data was sometimes read
from the wrong layer, too -- chat rows grouped by `session.workspace`, the
workspace's directory key, rather than its display name, so the group header
showed a path instead of the name a user picked in Settings.

The typed-query view already flattened groups into a per-row `Group ›` prefix
(`CommandPalette.vue`'s `displayList`, reading `cmd.group`), so a hand-built
title path was not needed there. Recent was the one place context actually
disappeared: its rows rendered an empty container prefix regardless of
`cmd.group`, so a recent feed or chat showed only its bare name -- and that loss is what
originally pushed a hand-built path into the title instead of fixing Recent's
own prefix.

## Decision

**A row's title names the thing; a row's group names what it lives inside.**
Which shape a row takes depends on what it is:

- A **command row**, seeded from `bindableCommands`, keeps the catalog's own
  verb title unchanged. `keymapRows` reads the same `command.title` for
  Settings › Keyboard, so a command's title has to read correctly standing
  alone in a list of shortcuts, not just as a palette row -- shortening it to
  an object noun there would break the editor it also has to serve.
- A **dynamic object row** -- a feed, Trash, a chat, a session, a window, a
  profile, a settings section -- sets `title` to the object's own
  name and `group` to its real container: the active profile's name for
  feeds/Trash, the workspace's display name for chats (`workspaceNameByDir`,
  falling back to the dir only when no workspace matched), the repo for
  session attach rows, the attached session's name for window rows, and the
  fixed `Settings`/`Profiles` labels for the rest. No verb prefix, no
  hand-built `A › B` string in the title.

Nesting is drawn once, by the palette's own rendering, never by a title.
`CommandPalette.vue` already had two places that show a row's context -- the
group header on an empty query, and the per-row `{{ groupPrefix }} ›` prefix
once a query narrows `results` past their headers -- plus Recent, which
reorders rows out from under their group but still renders through the same
`groupPrefix` field.
Keeping the container in `group` and letting these three read it is one
mechanism with one separator; if a title also carried a hand-built path, a
typed query would show it twice (`Container › acme/repo › main`) with no way
for rendering to know the title's own `›` was the same one it was about to
add.

The verb a title used to carry is not lost, it moves to `keywords`
('attach', 'switch', 'feed', 'chat', ...), so searching "switch acme" or
"attach repo" still ranks the row. `hint` stays reserved for the shortcut
combo (`hintFor`) or a numbered jump chord -- never a container, which is
`group`'s job alone. An object row also carries `kind` -- a muted type word
('window', 'feed') drawn at the right edge -- because the All view interleaves
bare-named objects with verb rows and a name alone does not say what Enter
does. `kind` names the object's type, never its container.

Group order still has to survive groups becoming dynamic strings: chats carry
`order: -2` and each active profile's feeds/Trash carry `order: -1` so the
browse view keeps the same section order (chats, then the active profile's
feeds block, then everything else) even though the group label itself now
varies per profile or per workspace.

## Consequences

A new dynamic object row sets `title` to the object's own leaf name and
`group` to its container, and does neither of: encoding the container into the
title, or prefixing the title with a verb. A verb that used to sit in the
title goes into `keywords` instead, and any container context that used to be
implicit in a hand-built title now has to be the real `group` value, not a
string assembled at the call site. A command row seeded from the catalog is
exempt -- its title stays the catalog's verb, because Settings › Keyboard
reads the same string.
