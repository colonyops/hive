# An orchestration workspace drives hive sessions through a token-checked MCP server

- **Status:** accepted
- **Date:** 2026-10-03

## Context

Work that spans repositories needs an agent above the sessions: one that
starts a hive session per repository, watches them, answers the ones blocked on
a prompt, and reports back (colonyops/hive#469). Nothing an agent could call
created a hive session, typed into one, or read the message bus. The app's MCP
servers were unauthenticated behind the loopback bind on the premise that
nothing on them spawns a process (ADR mcp-replaces-the-agent-facing-http-api),
and the terminal routes that do spawn are guarded by the frontend's token,
which an agent must not hold.

Driving a session by typing into its tmux pane was already in use from outside
the app, and it lost prompts: the agent drops an Enter that arrives while it
is still rendering the text, and a bare `tmux send-keys -t <session>` targets
the active window, which is usually the shell.

## Decision

**Session control is a third app-hosted MCP server, `hive-orchestrator`,** at
`/mcp/orchestrator`, beside `hive-desktop` and `hive-canvas`. A separate entry
keeps the grant visible: a workspace has session control exactly when its
`mcps:` list names this server.

**Every tool call authenticates with the calling chat's session token.** Each
agent-workspace launch already hands its process `HIVE_AGENT_SESSION_TOKEN`
(ADR a-scheduled-chat-ends-itself-through-a-capability-token-its-launch-handed-it).
The catalogue entry declares that variable as its bearer, and the generated
config names the variable rather than its value: `${HIVE_AGENT_SESSION_TOKEN:-}`
in the claude header, `bearer_token_env_var` for codex. No credential is
written into a workspace folder that syncs. The core resolves the token to its
chat and refuses the call unless that chat's workspace declares the server.
A client outside a workspace uses an access token created in Settings
instead. Hive stores only its SHA-256 and shows the plaintext once; a revoked
token stops working on the next call. Access tokens start with `hvo_`, which
is how the server tells the two kinds apart. `initialize` and `tools/list` need no token, because a 401 there sends Claude
Code looking for an OAuth server that does not exist.

**The tools report; the model decides.** The orchestrator is a model so that
it can read a screen and choose. The tools give it the screen and do what it
asks, and refuse nothing about what the agent shows. A guard in the tool is a
judgment that needs a code change to fix when it is wrong. A suggestion in
Claude Code's input box, read as unsent text, blocked every send until the
code learned about suggestions.

**Typing into a session is an engine service, `internal/hive/prompt`, used by the
CLI and the app.** It finds the agent pane through terminal discovery, waits
for the typed text to stop changing, presses Enter once, and returns the
screen. The wait is the fix for dropped Enters; whether the Enter was taken is
on the screen it returns. The CLI exposes it as `hive session send`, `keys`
and `peek`.

**Blocking tools answer within their timeout.** `wait_for_session`,
`wait_for_message` and `sleep` hold the request open, because a wait costs the
agent one call instead of a polling loop. The default is 50 seconds, under the
sixty some MCP clients allow a request, and the cap is four minutes, under the
five Claude Code lets an HTTP call stay silent. A wait that ends answers
`timedOut` and the agent calls again. Each also returns when the app shuts
down.

**The bus's read half is wired.** An orchestrator reads with acknowledgment
under the consumer `agentws.<workspace>`, so a fresh chat in the same workspace
does not re-handle what an earlier one took. `wait_for_message` acknowledges
what it returns before the answer is written, so an answer the client never
reads loses those messages; the default timeout keeps that window small.

**The seeded `orchestrator` workspace ships the `claude-ask` preset.** Every
session start and keypress then goes through the agent's own permission
prompt until the user loosens the command. Hive offers it to every
root once and records that in `.seeded-workspaces`, so an existing install gets
it, a deleted one stays deleted, and a directory already named `orchestrator`
is left alone. Its doctrine is its `AGENTS.md`, not a
shipped skill, because the shipped skills join every workspace that enables
the `hive` package.

## Consequences

- The "nothing on mcpsrv spawns" premise now holds for two of three servers.
  The third carries its own guard, and a new tool that spawns or types belongs
  on it.
- A token leaks only with the chat's environment, and it grants only what the
  chat's workspace declares. Deleting the chat retires it.
- The grant is read from the workspace manifest on every call, and a chat can
  edit its own manifest. The declaration is a boundary for the user to set,
  not a sandbox: any agent with a shell can also run `hive session send`.
  Binding the grant to the token at launch would close the first path and not
  the second.
- Sessions an orchestrator starts are tagged `orchestrator`, so its fleet is
  recoverable from hive without local state.
- Task tracking in `hive hc` is not wired into the server. An orchestrator
  that wants a durable plan runs the hive CLI from its shell.
