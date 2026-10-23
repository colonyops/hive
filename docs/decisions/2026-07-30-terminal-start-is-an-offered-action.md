# A session's terminal is started and killed on purpose, never as a side effect of attaching

- **Status:** accepted
- **Date:** 2026-07-30

## Context

A hive session's tmux session is not durable (a reboot, a killed tmux server, a
session created without one), while the session record is. An attach that
fails on a missing tmux session leaves a sidebar row that can only report
failure.

Spawning on attach, as the hive TUI does, is wrong here: spawning runs the
session's agent command, and the terminal view attaches without a click (it
re-attaches the remembered session on entering the mode). Opening the app would
start agents. What a session's terminal contains is already defined by hive's
spawn configuration, and re-deriving it in the desktop would be a second
definition.

## Decision

1. **Attach never spawns; `POST /api/terminal/start` does.** It reports
   `started` so a caller can tell "I created it" from "it was already running".

2. **A session that is not running is a classified answer.**
   `TerminalsService.Attach` probes `HasSession` and returns `KindNotFound`
   ("session %q is not running") before spawning a control client. The frontend
   branches on the `kind`.

3. **The UI offers it.** A stopped session shows a "Session not started" panel
   with **Start session**, and the session row menu carries **Start session**
   for an active session. Neither is offered for a recycled or corrupted
   session, which has no checkout.

4. **Killing is the same lifecycle from the other end.** `POST
   /api/terminal/kill` kills the tmux session and reports `killed`. The row
   menu's **Kill terminal…** asks for confirmation naming what stops (the
   agent) and what does not (the checkout, the work, the record). It is not
   Recycle or Delete. The control client is dropped synchronously with the
   kill, so an immediate re-attach cannot get the dying client. A slug tmux is
   not running answers `killed:false`.

5. **Creation goes through hive.** `SessionsService.StartTmuxSession` calls the
   engine's `OpenTmuxSession(…, targetWindow: "", background: true)`, so hive's
   own strategy resolution and templates decide the session's shape.
   Background, because the desktop attaches over control mode. Starting is
   idempotent.

6. **Policy stays in the session domain.** `StartTmuxSession` refuses a
   non-active session and a record whose name would slugify to something other
   than the slug being attached (ADR session-rename-keeps-slug-and-tmux-in-step).
   `TerminalsService` holds a one-method port.

## Consequences

- Nothing starts an agent without a click, including the resume path.
- The cold path costs one `has-session` per attach. A slug with a live control
  client answers from memory.
- Two concurrent starts on one slug can both probe "absent", and the loser
  reports a duplicate. Nothing serialises it, since starting is a deliberate
  click.
- A remote with a command-based spawn strategy (`spawn:`) cannot be started
  from the desktop; the panel reports hive's refusal. Attaching to one already
  running is unaffected.
