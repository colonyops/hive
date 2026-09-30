# `ptyterm` terminals are caller-addressed and concurrency-capped

- **Status:** accepted; [ADR agent-workspace-sessions-are-tmux-sessions](2026-08-03-agent-workspace-sessions-are-tmux-sessions.md) moved agent workspace sessions off `ptyterm` onto tmux — the caller-addressing and cap this ADR describes now govern the pop-up alone, and the cap counts live `agentws-*` tmux sessions instead of a `ptyterm.Manager` set
- **Date:** 2026-08-03

## Context

ADR ephemeral-popup-terminals gave `ptyterm` an id-keyed set of terminals, but `Open` (`manager.go`)
never let a caller choose the id — it always minted `t<N>` and handed it back.
That was enough for the pop-up: nothing about a shell you open for thirty
seconds wants to be addressed, and the frontend's one-pop-up-at-a-time policy
kept the count low regardless of a missing server-side cap.

The Agents area (spec-tracked as `hc-49x3i833`) needs both properties to
change. A workspace session is a named, durable record (spec §6.1): reopening
one must reattach to the terminal it started, not to whichever number the mint
counter happens to be on next, so the manager has to accept an id rather than
choose one. And the area removes the one-pop-up-at-a-time policy entirely —
nothing takes its place, and a session is an agent CLI that spawns a copy of
every enabled MCP server. Ten sessions opened over an afternoon is ten agent
processes and their MCP children, and an `npx -y` server entry refetches
remote code on every spawn. Without a cap that is a fork bomb with a progress
bar, not a feature.

## Decision

1. **`Spec` gains `ID`.** Empty still mints `t<N>` exactly as before — minting
   is unchanged and `TestOpenIsAlwaysANewTerminal` still passes unmodified. A
   non-empty `ID` is honoured: `Open` uses it as the terminal's address instead
   of minting one.

2. **A caller-supplied id already live is `ErrIDInUse`, not a second
   terminal.** The caller's id is what it will reattach with, so a collision is
   a rejection — the same way two `Open`s for a hive session's checkout are
   still always two terminals for the pop-up (`TestOpenIsAlwaysANewTerminal`),
   because *that* caller never repeats an id on purpose.

3. **`validateID` accepts `[A-Za-z0-9_.-]{1,64}` and rejects the minted shape
   `t<N>`.** The charset is what is safe as a map key today and, since a
   workspace session's id is durable, safe wherever a future caller folds it
   into a path. Rejecting `t<N>` as a caller id is what makes minting and
   caller-addressing safe to run side by side forever: a mint can never produce
   an address a caller is already holding, and a caller can never claim an
   address a future mint would produce.

4. **`maxConcurrentSessions = 8` is enforced in `Open`, across every caller.**
   The manager is the only place that knows the true count — the pop-up and a
   workspace session share one `Manager` instance — so the cap lives there
   rather than in either caller. The ninth `Open` returns `ErrTooManyTerminals`
   and spawns no process; closing one makes room for the next. There is still
   no idle reaping: the cap bounds concurrency, not lifetime.

5. **The stream is renamed, not duplicated.** `PopupTerminalStreamHandler`
   becomes `PTYStreamHandler`, mounted at `/api/terminal/pty/stream` in place
   of `/api/terminal/popup/stream`. The wire is unchanged — one terminal per
   socket, `0x00`/`0x01`/`0x10` frames — and it stays under
   `/api/terminal/` so the bearer token and CORS policy already guarding that
   prefix cover it without a second rule (ADR ephemeral-popup-terminals point 4). It is a rename
   because this is precisely the amendment that makes the stream address a
   terminal rather than a moment; a second identical mount for agent sessions
   would be the same wire under a name that lies about it.

6. **The pop-up keeps minting.** `PopupTerminalsService.Open` still passes no
   `ID` — nothing about a pop-up wanted to be addressable, and ADR ephemeral-popup-terminals's rule
   2 still holds for it unchanged. It does inherit the cap: eight live agent
   sessions mean the next pop-up `Open` returns `ErrTooManyTerminals`, riding
   the pop-up control plane like any other open failure.

### What still holds from ADR ephemeral-popup-terminals

- **Every terminal still dies with the app.** `App.Close` ends them all; a
  workspace session's *record* outliving the run does not make its *process*
  survive one.
- **There is still no shared interface with `tmuxcc`, and neither backend
  grows toward the other's job.** A session that needed to survive a restart
  would be a tmux session, not a `ptyterm` one reaching for durability it was
  never built to have.
- **The replay ring is still a raw byte ring**, not a reconstructed screen —
  it exists so a socket can be reopened against a still-running terminal, not
  so a screen can be rebuilt from history.

### What is amended

ADR ephemeral-popup-terminals point 2 said a terminal "is addressed by an id this process mints …
nothing about it is durable." That was true of every caller that existed. A
named workspace session is not a moment: its record is durable even though its
process is not, so the caller supplies the id and the manager honours it
rather than minting over it. ADR ephemeral-popup-terminals's "what is not built" list already noted
the manager holds an id-keyed set and that a second terminal is a UI decision —
this makes the id half of that explicit and gives it a validated shape.

## Consequences

- **A caller that knows its id in advance can reattach to a specific terminal**
  instead of having to remember whichever id `Open` returned. This is what
  phase 5's session launch needs and the pop-up does not.
- **`ptyterm` callers can no longer assume unlimited concurrency.** Eight is a
  shared budget; a caller that wants to open a ninth must close one first or
  surface the cap error, and the pop-up panel is one such surface now.
- **A caller-supplied id is validated input, not a trusted string.** Anything
  that turns a workspace name or a slug into a terminal id must produce
  something `validateID` accepts, or `Open` fails closed rather than minting a
  fallback.
