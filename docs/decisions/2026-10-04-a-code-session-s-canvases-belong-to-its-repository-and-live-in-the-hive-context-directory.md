# A Code session's canvases belong to its repository and live in the hive context directory

- **Status:** accepted
- **Date:** 2026-10-04

## Context

A canvas belonged to a Chats workspace: the `hive-canvas` entry was wired into
a workspace's generated MCP configuration, the caller named itself with the
`HIVE_AGENT_SESSION` id its launch was given, and the file went in the
workspace folder (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry).
An agent in a Code session had none of that (#505).

The desktop does not own a Code session's launch. The shared engine builds it
from the user's hive config, the CLI and the TUI start sessions too, and the
checkout is the user's repository. So the app can put nothing in the agent's
environment, cannot write an MCP file where the agent would read one, and has
no folder of its own beside the session.

## Decision

**The user adds the server to the agent's own MCP configuration.** Hive never
edits an agent's configuration outside a workspace. What it provides is a
stable address: the loopback port is written to `settings.yaml` the first
time it is allocated, and Settings ▸ MCP servers shows the address and the
setup for each agent.

**The `session` argument is a string, and it is one of two things.** A number
is a chat's record id, as before. Anything else is the caller's absolute
working directory, which the service resolves to the active hive session
whose checkout holds it. An agent wired up by hand has no id, but it always
knows where it runs, and the same rule works for a session the CLI started.

**A session's canvases belong to its repository.** They are filed at
`<context root>/<owner>/<repo>/canvases/<name>.json`, hive's per-repository
context directory, the one a checkout links as `.hive`. The owner is the
repository and not the session because hive gives a recycled session's id and
path to the next one, and because the session was the author, not the owner,
the conclusion the workspace decision reached about a chat. Every session of
a repository reads and writes the same canvases, as every chat of a workspace
does. `hive ctx prune` skips `canvases/`.

**The owner key is `owner/repo`, carried where a workspace name was.** A
workspace's key is one path component, so the slash keeps the two apart, and
the store, the read API and the frontend take either in the same field.

**Code and Chats show a canvas the same way.** The pane rides `?canvas` on
the terminal route as it does on the agents route, one title-bar toggle
opens both, and `open_canvas` applies to whichever is in view. The full-page
view lists repositories beside workspaces. A chat's pane shows its
workspace's canvases only.

## Consequences

- The tool schema changed: `session` was an integer. A chat whose agent
  holds the old tool list sends a number and is refused until it reconnects.
- Any local process can claim any directory, which is the standing the
  canvas server already had for a session id. What such a write can do is
  put a file in a `canvases/` directory and content in a pane.
- Canvases land in the context root, which for many users is a synced or
  versioned folder. They appear there as JSON files.
- The store restates hive's `<owner>/<repo>` layout, because it imports
  nothing. A test in the app package holds it to `config.RepoContextDir`.
- A remote that names no owner and repository has nowhere to file a canvas,
  and its session gets no pane.
