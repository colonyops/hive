# `ptyterm` terminals are caller-addressed and concurrency-capped

- **Status:** accepted
- **Date:** 2026-08-03

## Context

`ptyterm` (ADR ephemeral-popup-terminals) holds an id-keyed set of
terminals, and `Open` minted every id. A caller that must reattach to a known
terminal needs to choose its id, and every open terminal is a process the app
pays for, so the manager needs a ceiling. Agent workspace sessions were the
first such caller; they now run as tmux sessions instead
(ADR agent-workspace-sessions-are-tmux-sessions), so the pop-up is the only
`ptyterm` caller today.

## Decision

1. **`Spec.ID` is optional.** Empty mints `t<N>`. A non-empty id is used as
   the terminal's address.
2. **A caller-supplied id that is already live is `ErrIDInUse`**, never a
   second terminal.
3. **`validateID` accepts `[A-Za-z0-9_.-]{1,64}` and rejects the minted shape
   `t<N>`** (`ErrInvalidID`), so a mint can never collide with a caller id.
4. **`maxConcurrentSessions = 8` is enforced in `Manager.Open`** across every
   caller. The ninth `Open` returns `ErrTooManyTerminals` and spawns nothing.
   There is no idle reaping: the cap bounds concurrency, not lifetime.
   `AgentWorkspacesService` keeps its own cap of 8 (`maxConcurrentAgentSessions`)
   over live `agentws-*` tmux sessions on the same reasoning: each session is an
   agent CLI that spawns a copy of every enabled MCP server.
5. **The stream is `/api/terminal/pty/stream`** (`httpapi.PTYStreamHandler`):
   one terminal per socket, `0x00`/`0x01`/`0x10` frames, under
   `/api/terminal/` so the existing bearer token and CORS policy cover it.
6. **The pop-up keeps minting.** `PopupTerminalsService.Open` passes no `ID`
   and surfaces the cap error like any other open failure.

## Consequences

- `ptyterm` callers cannot assume unlimited concurrency; the budget of 8 is
  shared.
- A caller-supplied id is validated input. Anything that derives a terminal id
  from a name or slug must produce something `validateID` accepts, or `Open`
  fails closed.
