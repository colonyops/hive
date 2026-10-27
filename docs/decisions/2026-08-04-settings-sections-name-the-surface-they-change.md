# Settings sections name the surface they change, and shared values live in General

- **Status:** accepted
- **Date:** 2026-08-04

## Context

Application settings sectioned by kind (Appearance, System, Automation) stopped
saying what a control changes once the app had several surfaces. A control's
placement recorded which pane existed when it shipped, and a catch-all group
collected things with different owners.

## Decision

Application settings are cut by the surface a value changes. The nav groups
mirror the app's modes: Preferences, Inbox, Code, Chats, and Advanced
(`navGroups` in `SettingsView.vue`).

- A value only one surface uses lives on that surface's pane. A value several
  surfaces use lives in **General**; the editor command that "Open in editor"
  actions launch is the first.
- Integrations and Actions sit under Inbox because they are its input and
  output.
- There is no leftover group. A section that fits nowhere means the grouping is
  wrong.
- A launcher is a terminal, not an automation
  (ADR launchers-are-their-own-list-in-actions-yml), so it sits under Code,
  labelled **Quick terminals** to avoid colliding with the `launch-session`
  action executor. The `launchers:` key, the `launcher.<id>` command namespace,
  and the `/settings/launchers` route keep their names.
- **System** is this install on this machine: storage locations, diagnostics,
  and the problem reporter. A location owned by one surface (the agent-workspace
  root) lives on that surface's pane.
- **About** is its own pane, with the update controls, because the running build
  is not a setting.

This is UI placement only. No `settings.yaml` key moved.

## Consequences

- A pane's name is a claim about scope that review can check.
- Adding or renaming a section is three coordinated edits:
  `applicationSettingsSections` in `router.ts`, `applicationSettingsSectionMeta`
  in `components/settings/sectionMeta.ts`, and `navGroups` in
  `SettingsView.vue`.
- Error and empty-state copy that names a pane must move with it.
- A group with no settings of its own gets no pane; Inbox is only a group over
  its two panes.
