# Agent workspace sessions are tmux sessions

- **Status:** accepted; tmux naming amended by [ADR agent-workspace-tmux-sessions-use-persisted-random-ids](2026-09-21-agent-workspace-tmux-sessions-use-persisted-random-ids.md)
- **Date:** 2026-08-03

## Context

Phase 5 shipped agent workspace sessions on `ptyterm`, the same backend the
pop-up terminal uses (ADR ptyterm-terminals-are-caller-addressed): a caller-addressed PTY this process owns
outright, id `agentws-<record id>` (new sessions now use a persisted random id;
ADR agent-workspace-tmux-sessions-use-persisted-random-ids). Live use surfaced the defects that backend
cannot fix: a PTY has no concept of "the current screen at this client's
size," so a resize or a session switch replays whatever escape sequences the
scrollback happens to hold, mangling the redraw and interleaving output
between sessions. The phase-8 classifier attempt (spike hc-alqns469, commit
12ffe83) tried to work around this by reconstructing a screen from the raw PTY
ring tail through a VT emulator, and the spike falsified it: the projection
does not classify reliably. `terminal.Detector` — the classifier this
falsified attempt was trying to feed — is tuned against tmux's own
`capture-pane` output, not a hand-rolled emulation of one.

tmux already solves the rendering problem `ptyterm` cannot: an attach paints
the current screen at the client's size from tmux's own state (ADR terminal-first-paint-carries-scrollback), not
from replayed history. It also already solves persistence: a tmux session is
a server-side process tree that outlives any one client, including this one.

## Decision

1. **An agent workspace session is a tmux session, not a ptyterm one.** It is
   created detached — `tmuxcc.Manager.NewSession(ctx, name, dir, command)` —
   named `agentws-<terminal id>`, cwd the workspace directory, running the
   `agentws.Resolve`d command line through a login shell (the same
   shell-command-line contract `ptyterm.Spec.Command` used). It carries no
   hive session, no repo, and no row in hive's own session store — `agentws-*`
   is this service's own tmux namespace, addressed exactly like a hive slug
   but never mistaken for one.

2. **The pane rides the existing tmux stream, unchanged.** `tmuxcc.Manager` and
   `TerminalsService` were already slug-agnostic: `Attach`, `Subscribe`,
   `HasSession`, `KillSession` take any tmux session name, hive-known or not —
   nothing in that path validated the slug against hive's session list. The
   `/api/terminal/stream` WebSocket (`TerminalStreamHandler`) needed no
   change; `AgentsTransport` now points at it instead of the ptyterm stream
   (`PTYStreamPath`). There is still exactly one stream — no agent-specific
   mount was added.

3. **Start and Resume attach server-side.** The generic `/api/terminal/attach`
   control route is gated by `experimental.terminal`, which the Agents area
   (`experimental.agents`) must not depend on. `AgentWorkspacesService`
   therefore calls `tmuxcc.Manager.Attach` itself inside `StartSession` /
   `ResumeSession` and returns the active window's id (`SessionView.WindowID`,
   new field) alongside the session name — the frontend never calls
   `/api/terminal/attach`.

4. **Resume reattaches when the session is still alive; only a dead session
   relaunches.** `ResumeSession` probes `HasSession` first. Alive: `Attach`
   only — no relaunch, no fresh agent process — which is the actual
   persistence win: codex has no resume form (no `codex resume <id>` a fresh
   Hive run could reconstruct), so a codex workspace's only way to "reopen"
   the same conversation is the process never having stopped. Dead: relaunch
   fresh-or-resume exactly as before (`claude --resume` for agents with a
   resume form, a fresh session id for codex, with the same notice when a
   resume could not even be attempted).

5. **The 8-session cap is now a live tmux fact, not a manager's in-memory
   set.** `maxConcurrentAgentSessions` counts `tmuxcc.Manager.SessionNames(ctx,
   "agentws-")` at launch time rather than a map `ptyterm.Manager` owned — the
   same fork-bomb-with-a-progress-bar concern ADR ptyterm-terminals-are-caller-addressed point 4 raised, now
   against a backend where "how many are open" is answered by the process
   that actually owns them.

6. **`Available()` gates on tmux, mirroring `TerminalsService`.** Message and
   axis both changed: "agent workspaces need tmux 3.2 or newer" replaces the
   ptyterm build/platform check.

7. **SessionActivity (hc-ou4o02zx) returns to the proven classification path.**
   `AgentWorkspacesService.SessionActivity` runs `tmuxcc.Manager.CapturePane`
   (`capture-pane -t <name> -p -J`, the identical invocation
   `internal/hivecore/core/terminal/tmux.TmuxCapture` runs for hive's own
   sessions) through `dispatch.ClassifyAgentScreen`, a thin wrapper over
   `terminal.NewDetector(agent).DetectStatus` — the classifier the spike's
   falsifying fixtures were written against, fed the input it was actually
   tuned on. The in-flight VT-emulation `ProjectScreen` phase-8 was building
   is discarded, not merely paused. `POST
   /api/terminal/agents/sessions/activity` returns each live session's status
   (`ready` / `active` / `approval`); the Agents area polls it per workspace
   only while active, ~2s, and shows a per-session-row dot with approval as
   the highest urgency, matching `terminal.Detector`'s own IsBusy-wins,
   NeedsApproval-before-IsReady precedence.

8. **`autonomy: ask` is now the manifest default (hc-ou4o02zx §4).** ADR a-workspace-declares-its-own-authority
   point 6 made the M2 indicator a hard dependency of shipping `ask` as a real
   default; it has landed, so `agentws.Workspace.Validate` no longer requires
   `autonomy`, and an omitted value resolves to `AutonomyAsk` at load
   (`parseWorkspace`). `auto` and `full` remain explicit — nothing upgrades a
   workspace's posture by omission, only downgrades one to the safest choice.

## What still holds from ADR ephemeral-popup-terminals / 0060

- **Pop-ups are untouched.** `ptyterm` keeps running the pop-up terminal
  exactly as ADR ephemeral-popup-terminals/0060 describe — id-minted, caller-addressable up to the
  same 8-terminal cap, dying with the app. Nothing here grows either backend
  toward the other's job; an agent workspace session simply stopped being one
  of ptyterm's callers.
- **"Every ptyterm terminal dies with the app" (ADR ptyterm-terminals-are-caller-addressed) is still true of
  every terminal ptyterm now serves** — the pop-up. It is no longer true of an
  agent workspace session, because that session is not ptyterm's anymore.

## Consequences

- **Sessions outlive `App.Close` by design.** Quitting Hive no longer ends a
  running agent; the tmux session (and the agent process inside it) keeps
  running until explicitly closed (`CloseSession`/`KillSession`) or the
  machine's tmux server goes down. This is the persistence half of the
  tradeoff; the other half is that `full`-autonomy agents can now act
  unattended for as long as the session survives, not just for the life of
  one Hive run — the authority model (ADR a-workspace-declares-its-own-authority) already assumed a workspace's
  ceiling is its launch table, not the app's lifetime, so this does not
  change what a posture is allowed to do, only how long it can keep doing it
  unsupervised.
- **A workspace session's identity is a tmux session name, not a durable
  handle inside this process.** Restarting Hive, or even the machine's tmux
  server surviving a Hive restart, means `ResumeSession` finds the same
  `agentws-<terminal id>` session and reattaches to it — the reopen codex never had is
  now real for every agent.
- **`SessionView` gained `WindowID`.** A listing read (`Sessions`, `Open`)
  leaves it empty; only a call that attaches (`StartSession`, `ResumeSession`)
  sets it, since attaching is the only place a window is actually resolved.
