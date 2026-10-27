# Agent workspace sessions are tmux sessions

- **Status:** accepted
- **Date:** 2026-08-03

## Context

An agent workspace session on a raw PTY (`ptyterm`) has no notion of "the
current screen at this client's size", so a resize or switch replays whatever
escape sequences the scrollback holds, mangling the redraw. Classifying agent
status from a VT emulation of the PTY ring did not work reliably;
`terminal.Detector` is tuned against tmux's own `capture-pane` output.

tmux already paints the current screen at the client's size on attach (ADR
terminal-first-paint-carries-scrollback), and a tmux session outlives any one
client, this app included.

## Decision

1. **An agent workspace session is a tmux session.** It is created detached,
   named `agentws-<terminal id>` (ADR
   agent-workspace-tmux-sessions-use-persisted-random-ids), with the workspace
   directory as cwd, running the workspace's rendered command line through a
   login shell. It has no hive session, repo, or row in hive's session store;
   `agentws-*` is this service's own tmux namespace.

2. **The pane rides the existing tmux stream.** The terminal service and
   `tmuxcc.Manager` take any tmux session name, so the agent pane uses the same
   `/api/terminal/stream` WebSocket as session terminals, with no agent-specific
   mount.

3. **Start and Resume attach server-side.** `AgentWorkspacesService` attaches
   inside `StartSession` and `ResumeSession` and returns the active window and
   pane (`SessionView.WindowID`, `PaneID`). A listing read leaves them empty.

4. **Resume reattaches when the session is alive; only a dead session
   relaunches.** `ResumeSession` probes `HasSession` first. That is the
   persistence win: an agent with no resume form (codex) can only reopen a
   conversation whose process never stopped. A dead session relaunches with
   the command template's resume form when it has one.

5. **The 8-session cap counts live tmux sessions.**
   `maxConcurrentAgentSessions` counts `SessionNames(ctx, "agentws-")` at
   launch, so the count comes from the process that owns the sessions.

6. **`Available()` gates on tmux**, with the same tmux 3.2 requirement as
   session terminals.

7. **Activity uses the proven classifier.** `SessionActivity` captures each
   live pane and classifies it with the agent's `terminal.Detector` into
   `ready`, `active` or `approval`. `POST
   /api/terminal/agents/sessions/activity` returns it, and the Agents area
   shows approval as the highest urgency.

## Consequences

- Sessions outlive `App.Close`. Quitting Hive does not end a running agent;
  the tmux session runs until closed or the tmux server goes down. An agent
  with permission bypasses can act unattended for as long as the session
  survives, which is the ceiling ADR a-workspace-declares-its-own-authority
  already assumes.
- A restart of Hive finds the same `agentws-<terminal id>` session and
  reattaches to it.
- `ptyterm`'s only remaining caller is the pop-up terminal (ADR
  ephemeral-popup-terminals).
