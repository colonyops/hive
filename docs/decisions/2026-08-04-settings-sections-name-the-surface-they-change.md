# Settings sections name the surface they change, and shared values live in General

- **Status:** accepted
- **Date:** 2026-08-04

## Context

Application settings were sectioned by kind — Appearance, System, Automation —
which stopped answering "what does this control change?" once the app grew more
than one surface. Appearance carried the terminal typography that the Code tab,
the pop-up panel and the Agents area's chats all draw with; System carried the
editor command that open-in-editor actions launch, along with storage
locations, diagnostics, an Experimental list and an About card. A control's
placement recorded which pane happened to exist when it shipped.

Three forces made that worse rather than merely untidy. The editor command is
about to have consumers across the Inbox, the Code tab and the Agents area
(#224, #225), so filing it under whichever surface shipped first is now wrong
in a way a reader can observe. Every ships-dark opt-in landed in one
"Experimental" list, which grouped features by how finished they are instead of
by what they do — a category that empties itself as features graduate. And an
"Automation" group had accumulated Integrations, Actions, Launchers and Skills,
four things with four different owners: it was not a category, it was what was
left over.

## Decision

Application settings are cut by **the surface a value changes**, and the nav
mirrors the app's own mode switch:

```
Preferences  General · Appearance · Notifications · Keyboard
Inbox        Integrations · Actions
Code         Terminal · Quick terminals
Agents       Agents · Skills
Advanced     System · About
```

- A value only one surface uses lives on that surface's own pane, named for the
  surface. A value several surfaces use lives in **General**. The editor
  command is the first: it is shared rather than agents-owned, so it gets one
  clearly-named home instead of being surfaced in three places.
- **The group titles are the modes** — Inbox, Code, Agents — so the nav can be
  read against the title bar rather than learned. Integrations and Actions sit
  under Inbox because they are its input and output: a connector feeds the
  pipeline, and an action's default target is `item`, the feed item's own menu.
- **There is no leftover group.** "Automation" held four differently-owned
  things and named none of them. A section that fits nowhere is evidence the
  grouping is wrong, not licence to open a bucket.
- **A launcher is a terminal, not an automation.** ADR launchers-are-their-own-list-in-actions-yml established that a
  launcher is not an action and shares only a file with one; it opens the pop-up
  terminal into a program, behind the same opt-in as the Code tab, and
  contributes a `launcher.<id>` command. It sits under Code. It is also
  relabelled **Quick terminals**, because "Launchers" collided with the
  `launch-session` action executor — which does launch agent sessions, and is a
  different thing entirely. The `launchers:` key, the `launcher.<id>` command
  namespace and the `/settings/launchers` route keep their names: the collision
  was in what the user reads, not in what the code calls it.
- **System** is this install on this machine — storage locations, diagnostics,
  and the problem reporter that bundles them. Not a junk drawer for anything
  with a path: the agent-workspace root is a location, and it lives on Agents.
- **About** is its own pane, because the running build is not a setting. The
  auto-update controls sit with it, where "what am I running" is answered.
- **Experimental is a posture, not a category.** A ships-dark opt-in (ADR terminal-experimental-gate)
  renders on the pane for the feature it gates, carrying an Experimental badge
  and its restart-pending state (`settings/ExperimentalToggle.vue`). There is
  no Experimental section to graduate out of.

`/settings/system` keeps working: System survives the re-cut as a real section
with a narrower charter, so no route is retired and no bookmark breaks.

The YAML is untouched. `editor` was already a top-level key in `settings.yaml`,
so this is a UI-placement change and needs no forward-only migration (ADR yaml-config-migration).

## Consequences

- A pane's name is a claim about scope, and reviews can check it: a control
  whose effect is not what the pane is named for belongs elsewhere. "Which pane
  does this go on?" has an answer that does not depend on shipping order.
- Adding or renaming a section stays three coordinated edits —
  `applicationSettingsSections` in `router.ts`, `categoryMeta` and `navGroups`
  in `SettingsView.vue` — and `SettingsView.spec` still fails a partial one.
- Error and empty-state copy names a pane, so moving one is a code change with
  a search cost. That is the price of wayfinding that is true; the strings are
  few and a stale one is a bug, not a cosmetic drift.
- A surface with a single setting still gets a pane (Agents is one opt-in and
  one path today). A pane that would hold nothing does not get created — there
  is no Inbox *pane*, only an Inbox group over the two panes that serve it.
- Appearance is down to the theme picker and General to the editor command.
  They stay separate rather than merging: Appearance is the entry users look
  for first in any app, and the editor's whole point is a home of its own.
- The editor command is a combobox rather than a list of the four detected
  launchers, so any single-word command can be set without hand-editing
  `settings.yaml`. It persists on a debounce, since every keystroke emits and
  each write is a read-modify-write of the file.
