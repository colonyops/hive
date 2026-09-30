# The open chat rides the route, and re-entry reattaches it if live

- **Status:** accepted
- **Date:** 2026-08-04

## Context

The Agents pane's attachment — which chat is open — lived only in webview
memory, so a reload or relaunch always came back to an idle pane even though
the session itself, being a tmux session (ADR agent-workspace-sessions-are-tmux-sessions), was still running.
Terminal mode already puts its whole surface in the URL (`/terminal/:slug`,
`?window`) so history traversal and mode re-entry restore it; the agents
route carried only the focus filter. The standing rule was that nothing
attaches unprompted on a fresh load: re-entering a mode is what resumes it.

The complication is that `ResumeSession` is two different actions behind one
call: for a live tmux session it only attaches, but for a dead one it
*relaunches* the agent. An automatic resume on reload must never be the thing
that starts a process.

## Decision

The chat open in the pane is named in the route as `?chat=<id>` on
`/workspaces/:workspace?`, kept in sync declaratively: the pane going live
writes the id, the pane going idle (exit, failure, close) removes it, and
moving the focus filter carries it along — the two are independent axes.
Since the mode toggle's remembered agents path persists across reloads
(localStorage), the query survives both a webview reload and a relaunch.

Re-entering the area with `?chat` present auto-resumes the chat **only if
the sessions listing reports its tmux session live** (`terminalId` set — the
listing probes tmux per row). A dead or unknown chat's param is stripped
instead: resuming it would relaunch, and a relaunch stays a deliberate click
on the row. The beat between listing and resuming, in which the session
could die and the resume relaunch it, is an accepted race.

## Consequences

- A reload lands back in the same workspace with the same chat attached; the
  "never attach unprompted" rule is softened to "reattach what was
  attached" — an attach can now follow a reload, a process launch never can.
- The launch default is unchanged: a relaunch still lands on the hub, and
  only the Workspaces toggle carries the stored path back into the area.
- The chat's presence in the URL makes it history-traversable, but the pane
  is deliberately conservative: `?chat` is only acted on when the pane is
  idle, so history moves never tear down a live chat.
