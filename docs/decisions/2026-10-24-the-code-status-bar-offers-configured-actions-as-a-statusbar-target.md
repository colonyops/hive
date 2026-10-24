# The Code status bar offers configured actions as a statusbar target

- **Status:** accepted
- **Date:** 2026-10-24

## Context

The Code status bar had fixed buttons: editor, file manager, Tasks. Users
wanted their own ("open the repository", "open CI"), and the actions model
already runs a user-defined command against a session
(ADR actions-target-terminal-sessions-and-windows). Its decision 7 put session
actions in the row menu only.

## Decision

1. **`statusbar` is a fourth value of `targets:`.** A status bar action runs
   against the session exactly as a `session` action does: same
   `TerminalTarget` with no window id, same template data, same jobs path.
   `InvokeTerminalAction` admits a session invocation for an action that
   declares either surface. `launch-session` cannot declare it, for the reason
   it cannot declare `session`.

2. **An action carries an optional `icon:`** from the launcher glyph set
   (ADR launchers-are-their-own-list-in-actions-yml), and only the status bar
   draws it. The label is the button's tooltip.

3. **Configured buttons come after the built-in ones.** The editor, file
   manager, and Tasks buttons are not configurable and are not
   actions. A config file adds to the bar; it cannot remove or reorder what
   ships. Opening the repository is not built in: it is a one-line
   `statusbar` action over `.Session.WebURL`.

4. **`.Session.WebURL` is the repository's web page, resolved by the forge
   that serves the remote.** A connected Gitea instance answers with its own
   base URL, because an SSH remote names git's port and no remote names the
   subpath an instance is served under. A host no forge serves falls back to
   an address read off the remote (`git.WebURL`). There is no `open-url`
   action type: `shell` with `open {{ .Session.WebURL | shq }}` covers it.

## Consequences

- A status bar action shows on every session's bar. There is no per-host or
  per-repository filter, so a forge-specific path in a template is wrong on a
  session whose remote is on another forge.
- Each click on a `shell` button is a job in the jobs UI, including one that
  only opens a URL.
- The launcher glyph set now serves two surfaces, and stays in sync with
  `frontend/src/lib/launcherIcons.ts` by hand.
