# The open chat rides the route, and re-entry reattaches it if live

- **Status:** accepted
- **Date:** 2026-08-04

## Context

A chat's session is a tmux session that outlives a webview reload
(ADR agent-workspace-sessions-are-tmux-sessions), so the open chat should
survive one too. `ResumeSession` is two actions behind one call: for a live
tmux session it attaches, for a dead one it relaunches the agent. A reload must
never start a process.

## Decision

The chat open in the pane is named in the route as `?chat=<id>` on
`/workspaces/:workspace?`. The pane going live writes the id; the pane going
idle removes it. The workspace focus and the chat are independent, and the mode
toggle's remembered path keeps the query across reloads and relaunches.

When the route names a chat other than the open one, the pane switches to it,
but only if the sessions listing reports its tmux session live (`terminalId`
set). Otherwise the param is reset to whatever the pane holds: a relaunch stays
a deliberate click on the row. An in-flight launch is never interrupted. The
gap between listing and resuming, in which the session could die and the resume
relaunch it, is an accepted race (`AgentsMode.vue`).

## Consequences

- A reload can attach a chat; it never launches one.
- Closing a chat also drops `?doc`, which names a view of the open chat.
