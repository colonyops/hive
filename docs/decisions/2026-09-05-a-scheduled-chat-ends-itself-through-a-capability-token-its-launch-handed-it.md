# A scheduled chat ends itself through a capability token its launch handed it

- **Status:** accepted
- **Date:** 2026-09-05

## Context

A scheduled chat is an ordinary interactive agent launch in tmux. When the
agent finishes its task the CLI sits at its prompt and the tmux session stays
alive until a person closes it, and while it does the schedule's next
occurrence is skipped as "the previous run's chat is still running". A job
nobody closes never fires again. The agent had no sanctioned way to end its
own session: the loopback MCP server excludes session control on purpose, and
the HTTP close route wants the frontend's per-run terminal token, which the
agent does not hold.

## Decision

**The agent ends its own session, with a capability its launch handed it.**
Every launch mints a token, stores it on the session row, and hands the
process `HIVE_AGENT_SESSION_TOKEN` and `HIVE_AGENT_SESSION_END_URL`.
`POST /api/sessions/end` with that token as the bearer ends that session and
no other. It is a base HTTP route with its own guard rather than an MCP tool
because a workspace need not declare the app's MCP server for its schedules
to work, and a `curl` from the agent's shell needs no tool wiring at all. It
does not join the `/api/terminal/` prefix because that prefix's guard is the
frontend's token, and a chat must not hold it.

**A scheduled chat is told to end itself.** The scheduled prompt is framed: a
prefix naming the schedule and workspace and saying nobody is watching, the
schedule's own rendered prompt, and a suffix that says the agent MUST end the
session when done and gives the exact command. The command reads two
environment variables and carries no JSON, so there is nothing for the agent
to quote. The run history keeps the unframed prompt. The frame is Go-owned
prompt text in `internal/app/prompts/templates`.

**The chat is deleted after a grace, not inside the call.** The request
arrives from inside the agent's own tool call; killing the pane there would
cut the tool result, and any closing message, out of the transcript. The route
answers 202 with when the chat will be gone, and deletes it after
`agent_workspaces.session_end_delay` (10 seconds shipped). Waiting for the
agent to read as idle was considered and rejected as more machinery than the
problem needs.

**Deleted, not closed.** The record goes with the tmux session. A schedule
that runs hourly would otherwise leave a row per run in the sidebar, and the
run history already keeps what a person wants afterwards: the prompt, the
outcome and any error. A history entry drops its chat pointer once the chat is
gone, so nothing offers to open a chat nobody can.

**Interactive mode stays.** Launching scheduled runs headless (`claude -p`,
`codex exec`) would end the process by itself, but loses the pane while it
runs and cannot take the `ask` autonomy; the chat pane and the resumable
conversation are the point of a scheduled chat.

## Consequences

- A schedule's next occurrence is no longer blocked by its previous chat,
  provided the agent obeys the frame. An agent that does not call the route
  leaves the chat open exactly as before, and the run history shows the skip.
- A hand-started chat can end itself too, if told to: every launch is handed
  the token and the URL. Ending is deleting, for that chat as well.
- The conversation itself is not kept by Hive once the chat is gone; anything
  a scheduled run produces that is worth keeping belongs in a canvas or a
  file, and the frame says so.
- `agent_workspace_session` gains `end_token`, in the unreleased migration
  0008.
- When the loopback HTTP server is off there is no URL to hand out, and the
  frame omits the closing instruction rather than pointing at nothing.
