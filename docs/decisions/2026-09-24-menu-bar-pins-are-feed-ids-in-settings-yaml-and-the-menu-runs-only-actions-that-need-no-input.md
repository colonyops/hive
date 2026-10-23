# Menu bar pins are feed ids in settings.yaml and the menu runs only actions that need no input

- **Status:** accepted
- **Date:** 2026-09-24

## Context

The menu bar shows a few priority feeds, each item with an actions submenu.
A pinned list is ordered across profiles, so it belongs to no single flow
file. A native menu cannot show an action's input form, the New Session
dialog, or a rerun confirmation.

## Decision

Pins are `menu_bar.feeds` in `settings.yaml`: a list of feed ids
(`<profile>/<feed node>`) with an optional per-feed item limit, capped at
three feeds and ten items. A pin that names no loaded feed is skipped when
read, like `profiles.order`.

The pinned feeds are the menu's only buckets. An item row shows its title and
an unread marker, and the menu shows no counts.

An item's submenu offers an action only when it is shown in detail, applies
to the item, and runs without input (`Action.RunsWithoutInput`: headless
capable, or a clipboard render whose inputs all have defaults). A rerun
that needs confirmation, or a failed run, opens the item in Hive.

`app.MenuBarService` builds the snapshot, and the tray in `wailsui` renders
it, rebuilding on core events and when the poller's last tick moves.
Profiles appear as links that open the profile in Hive.

## Consequences

Pins sync with the rest of `settings.yaml`. A feed whose id changes drops out
of the menu until it is pinned again. Actions that need input are reachable
only from the main window.
