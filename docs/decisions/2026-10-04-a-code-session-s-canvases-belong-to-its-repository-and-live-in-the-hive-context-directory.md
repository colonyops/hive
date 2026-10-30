# A Code session's canvases belong to its repository and live in the hive context directory

- **Status:** accepted
- **Date:** 2026-10-04

## Context

The desktop does not own a Code session's launch. The engine builds it from
the user's hive config, the CLI and the TUI start sessions too, and the
checkout is the user's repository. So the app can put nothing in the agent's
environment and cannot write an MCP config where the agent would read one.

## Decision

**The user adds the server to the agent's own MCP configuration.** Hive never
edits an agent's configuration outside a workspace. It provides a stable
address instead: the loopback port is written to `settings.yaml` the first
time it is allocated, and Settings ▸ MCP servers shows the address and the
setup for each agent.

**The `session` argument is a string.** A chat passes its
`HIVE_AGENT_SESSION` id, and an agent outside every session passes `global`
(ADR agents-outside-every-session-share-one-global-canvas-owner). Anything
else is the caller's absolute working directory, which
`DriveService.resolve` maps to the active hive session whose checkout holds
it. An agent wired up by hand has no id, but it always knows where it runs.

**A session's canvases belong to its repository.** They are filed in the
repository's drive space, `<context root>/<owner>/<repo>/`, the directory a
checkout links as `.hive`
(ADR hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it).
The owner is the repository and not the session, because hive gives a
recycled session's clone to the next one. Every session of a repository
reads and writes the same canvases. Prune never removes a canvas.

**The owner key is `owner/repo`**, in the same field that carries a
workspace name. The slash keeps it apart from a workspace key.

**Code and Chats show a canvas the same way.** The same drive pane sits beside
a chat and a Code session, and `open_canvas` applies to whichever is in view.

## Consequences

- Any local process can claim any directory. What such a write can do is put
  a file in a repository's space and content in a pane.
- Canvases land in the context root, which for many users is a synced or
  versioned folder.
- A remote that names no owner and repository has nowhere to file a canvas,
  and its session gets no pane.
