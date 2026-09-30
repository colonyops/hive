# A session's terminal is started and killed on purpose, never as a side effect of attaching

- **Status:** accepted
- **Date:** 2026-07-30

## Context

`POST /api/terminal/attach` ran `tmux -C attach -t <slug>` and failed when
nothing answered to that name. A hive session's tmux session is not durable —
the machine reboots, the tmux server is killed, a session is created by an
action that never spawned one — while the session record is, so every one of
those left a sidebar row that could only ever report an attach failure. The hive
TUI has no such state: selecting a session there opens it, creating the tmux
session first if it has to.

Copying the TUI exactly would mean attach spawns implicitly. It was built that
way first and rejected: spawning runs the session's **agent command**, and the
terminal view attaches without an explicit click — the remembered session is
re-attached on entering the mode (ADR terminal-attach-pool). "Open the app" would have started
agents.

What a session's terminal contains is already specified: hive's spawn
configuration for the remote (`windows:` under a matching rule, otherwise
`DefaultWindows`), rendered against the session's path, name, slug, context dir,
owner and repo. Re-deriving any of that in the desktop would be a second
definition of a session's shape, drifting from the one `hive` itself uses.

## Decision

1. **Attach never spawns; `POST /api/terminal/start` does.** Start is a separate
   operation on the terminal control plane, reporting `started` so a caller can
   tell "I created it" from "it was already running". Attach and start are one
   click apart in the UI and one call apart for an agent.

2. **A session that is not running is a classified answer, not a dead stream.**
   `TerminalsService.Attach` probes `Manager.HasSession` and returns
   `KindNotFound` ("session %q is not running") before it spawns a control
   client. Previously that case surfaced as tmux's own complaint wrapped in
   `KindInternal` — unactionable, and unclassifiable without matching error text
   (PR rule 4). The frontend branches on the `kind` on the wire.

3. **The UI offers it in the main panel and the row menu.** Selecting a stopped
   session shows a "Session not started" panel where the terminal would be, with
   a primary **Start session** button; the session row's ⋯ menu carries a
   **Start session** entry for an active session, which starts and then attaches.
   Neither is reachable for a recycled or corrupted session — there is no
   checkout to open a terminal in.

4. **Killing is the same lifecycle from the other end.**
   `POST /api/terminal/kill` kills the tmux session and reports `killed`; the
   row menu's **Kill terminal…** sits beside Start, behind a confirmation naming
   what stops (the agent) and what does not (the checkout, the work, the
   record). It is deliberately not Recycle or Delete: neither the hive record
   nor the directory is touched, and the session can be started again from the
   panel a kill leaves behind. The control client is dropped synchronously with
   the kill, so a re-attach in the same breath cannot be handed the dying
   client's window set. A slug tmux is not running answers `killed:false` rather
   than failing — killing what is already gone succeeded, the same tolerance
   detach has.

5. **Creation goes through hive, via the ACL seam.**
   `dispatch.HiveSessionManager.SpawnTmuxSession` calls hive's
   `SessionService.OpenTmuxSession(…, targetWindow: "", background: true)` —
   its own strategy resolution, template rendering and tmux client. Background
   because the desktop attaches over control mode; an attaching spawn would hand
   the session to whatever terminal launched the app, or fail for a launcher
   with none. `OpenTmuxSession` skips a session tmux already holds, and `Start`
   probes first anyway, so starting is idempotent.

6. **Policy stays in the session domain.** `SessionsService.StartTmuxSession`
   resolves the slug to a session, refuses a non-active one and refuses a record
   whose name would slugify to something other than the slug being attached
   (ADR session-rename-keeps-slug-and-tmux-in-step's invariant — hive spawns under the slug it derives from the name).
   `TerminalsService` holds a one-method port and none of that knowledge.

7. **The same tmux server, by construction.** Hive's spawn runs with `$TMUX`
   intact and the control client passes `-S` from that same value (ADR tmux-discovery's
   binary, ADR terminal-transport's client), so create and attach address one server whether
   or not the app was launched from inside tmux.

## Consequences

- No sidebar row is dead: every one either attaches or offers to start. Nothing
  starts an agent without a click, including the resume path.
- The cold path costs one extra `has-session` fork per attach. A slug with a
  live control client answers from memory, so warm re-attaches and the pooled
  switches of ADR terminal-attach-pool pay nothing.
- Two concurrent starts on one slug can both probe "absent", and the losing
  `new-session` reports a duplicate. Nothing serialises it: starting is a
  deliberate click, and the panel reports the failure with the button still
  there.
- A remote whose spawn strategy is command-based (`spawn:`, typically launching
  a GUI terminal) cannot be started from the desktop; the panel reports hive's
  own refusal. Attaching to one that is already running is unaffected. Making
  that message better would mean resolving the strategy here, which is the
  duplication decision 4 exists to avoid.
- `SessionManagement` gained `OpenTmuxSession`, so an upstream signature change
  breaks `hive_adapters.go` rather than the core.
