# A scheduled chat ends itself through a capability token its launch handed it

- **Status:** accepted
- **Date:** 2026-09-05

## Context

A scheduled chat is an ordinary interactive agent launch in tmux. When the
agent finishes, the session stays alive until a person closes it, and while
it does the schedule's next occurrence is skipped as still running. The
agent needs a sanctioned way to end its own session. The loopback MCP server
excludes session control on purpose, and the terminal routes take the
frontend's token, which the agent must not hold.

## Decision

**The agent ends its own session with a capability its launch handed it.**
Every launch mints a token (`end_token` on the session row) and hands the
process `HIVE_AGENT_SESSION_TOKEN` and `HIVE_AGENT_SESSION_END_URL`.
`POST /api/sessions/end` with that token as the bearer ends that session and
no other. It is a base HTTP route with its own guard, not an MCP tool, so a
`curl` from the agent's shell works whether or not the workspace declares
the app's MCP server. It stays outside the `/api/terminal/` prefix because
that prefix's guard is the frontend's token.

**A scheduled chat is told to end itself.** The scheduled prompt is framed
by `prompts/templates/scheduled-run.tmpl`: a prefix naming the schedule and
workspace and saying nobody is watching, the schedule's rendered prompt, and
a suffix with the exact command. The command reads only the two environment
variables, so there is nothing for the agent to quote. The run history keeps
the unframed prompt.

**The chat is deleted after a grace, not inside the call.** The request
comes from inside the agent's own tool call, and killing the pane there would
cut the tool result out of the transcript. The route answers 202 with when
the chat will be gone and deletes it after `agent_workspaces.session_end_delay`
(10s default).

**Deleted, not closed.** The record goes with the tmux session, so an hourly
schedule does not leave a sidebar row per run. The run history keeps the
prompt, outcome and error, and drops its chat pointer once the chat is gone.

**Interactive mode stays.** A headless launch would end by itself but loses
the pane and the resumable conversation, which are the point of a scheduled
chat.

## Consequences

- An agent that does not call the route leaves the chat open, and the run
  history shows the next occurrence as skipped.
- Every launch gets the token, so a hand-started chat can end itself too,
  and for it ending is also deleting.
- Hive does not keep the conversation once the chat is gone. Output worth
  keeping belongs in a document or file, and the frame says so.
- When the loopback HTTP server is off there is no URL, and the frame omits
  the closing instruction.
