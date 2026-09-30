# Menu bar pins are feed ids in settings.yaml and the menu runs only actions that need no input

- **Status:** accepted
- **Date:** 2026-09-24

## Context

Issue #525 asks for a few priority feeds in the menu bar, each item with an
actions submenu. A pinned list is ordered across profiles, so it belongs to no
single flow file. Hive has no priority classification, so the issue's
"need you / low priority / noise" split has nothing to count. A native menu
cannot show the action input form, the New Session dialog, or the rerun prompt.

## Decision

Store pins as `menu_bar.feeds` in settings.yaml: a list of feed ids
(`<profile>/<feed node>`) with an optional per-feed item limit. Cap the list at
three and the limit at ten. A pin that names no loaded feed is skipped when
read, like `profiles.order`.

The feeds themselves are the buckets; unpinned feeds do not appear in the
menu. The menu is an at-a-glance view: an item row shows its title and an
unread marker, and the menu shows no counts.

The item submenu offers an action only when it is shown in detail, applies to
the item, and needs no input: it is headless-capable, or a clipboard render
whose inputs all have defaults. A rerun that needs confirmation, or a failed
run, opens the item in Hive.

`app.MenuBarService` builds the snapshot. The tray in `wailsui` renders it and
rebuilds on inbox, flows, actions and pin changes, and when the producer's
last tick moves. Profiles appear as links that open the profile in Hive, not
as enable toggles.

## Consequences

Pins sync with the rest of settings.yaml. A feed renamed by id drops out of the
menu until it is pinned again. Actions that need input are reachable only from
the main window.
