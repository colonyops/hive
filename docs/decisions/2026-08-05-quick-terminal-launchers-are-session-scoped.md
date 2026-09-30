# Quick terminal launchers are session-scoped

- **Status:** proposed; the directory a launcher resolves is amended by [ADR a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is](2026-08-29-a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is.md) — it is the terminal's active pane rather than the session's checkout, which is what makes a launcher work on the scratch terminal and on a pinned chat
- **Date:** 2026-08-05

## Context

ADR launchers-are-their-own-list-in-actions-yml made each launcher a bindable command with a global context,
and ADR ephemeral-popup-terminals gave the pop-up a directory resolution that ends in the user's home. The
two compose into a launch nobody wants: `lazygit` bound to a chord, pressed on
the feed, opens a PTY in `$HOME`, and the program inside exits on the first
frame because there is no repository under it. The terminal opened; the launch
failed. `App.vue` only ever supplied a session slug when a Terminal route was on
screen, so every other surface took the fallback.

The fallback is right for the bare interactive shell — "a terminal, here,
now" has an answer with no session in it. It is wrong for a configured launcher,
whose command was written against a checkout.

## Decision

**A launcher with no configured `cwd` runs in a session's checkout or not at
all.** `cwd` is what makes a launcher self-contained; without one the session is
the only thing that says where its command runs, so a launch with no session to
resolve is refused rather than redirected. A launcher that pins a `cwd` is
unchanged and stays reachable from anywhere.

The core is where that is enforced. `PopupTerminalsService.applyLauncher`
refuses a session-scoped launcher with no slug (`KindInvalid`), and drops the
caller's `Dir`: the directory is the session's answer, and honouring a path
beside the slug would be a second way to answer the same question — the bypass
the enforcement exists to close. A slug whose session is gone already fails as
`KindNotFound` through `SessionsService.SessionDirectory`, and a recycled one as
`KindConflict`; neither falls back. This holds for the Wails frontend and for
any HTTP API caller, which is the point of putting it here rather than in
`App.vue`.

`PopupLauncher` carries `requiresSession` so the frontend gates on the core's
own answer instead of re-deriving it from a `cwd` it is deliberately not told.
It becomes the command's context — a new `terminal-session` member, active only
in terminal mode with a session attached — so the keymap and the palette both
gate through the machinery that already existed. Outside that context the chord
is not dispatched (it falls through to whatever else would have taken it, rather
than being swallowed) and the palette row is not listed.

## Consequences

A launcher's reach is now a consequence of its `cwd`, which is one more thing
`cwd` means than it did. The launcher editor and `launchers.md` say so, because
nothing else in the config makes it visible.

The palette hides a session-scoped launcher rather than showing it disabled with
a reason. Both were acceptable; hiding needs no disabled-row affordance in
`useCommands`/`CommandPalette.vue`, and Settings ▸ Quick terminals is where a
configured launcher is always visible and always explains itself. If disabled
rows arrive for another reason, this is a good first user of them.

`terminal-session` is a command context nothing in the static catalog claims —
it exists for commands loaded from config. A built-in that needs the same gate
should use it rather than adding a second spelling.

The chord that opens a launcher no longer closes its pop-up after a walk back to
the hub, because outside the session it is not dispatched at all. The panel's
own hide and end controls are still there, and a launcher pop-up outliving the
session it was opened from is not a case worth a carve-out in two dispatch
paths.

The bare pop-up shell keeps the home fallback. It is not a configured command
and has nothing to fail at.
