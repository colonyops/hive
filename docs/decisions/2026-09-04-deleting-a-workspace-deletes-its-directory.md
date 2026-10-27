# Deleting a workspace deletes its directory

- **Status:** accepted
- **Date:** 2026-09-04

## Context

The workspace list is a scan of the root for directories that hold an
`agent-workspace.yaml` (`agentws.Store`). A delete that leaves the manifest
on disk is undone by the next scan (#354). Either the delete removes what
the scan reads, or app state starts overruling the scan.

## Decision

Delete removes the directory. `AgentWorkspacesService.DeleteWorkspace` kills
every live terminal the workspace's sessions hold, deletes the directory and
everything under it, then removes the session records and schedule state and
reloads the store.

The order decides what a failure leaves behind. Terminals go first because
their working directory is about to disappear. The directory goes before the
records, so a delete that cannot finish still has the chat history to come
back to.

`agentws.RemoveWorkspace` removes files it did not write, so it re-checks its
target: `dir` must be a single path element local to the root, and must name
a directory that holds a manifest. The root itself, `.shared`, `mcps.yaml`,
and a symlink fail those checks. The service also refuses a directory the
root does not list as a workspace.

An archive flag was rejected: a tombstone is app state deciding what a
directory tree contains, and it would have to travel between machines that
share a synced root.

The confirm states the full path it is about to delete and what goes with
it. There is no undo.

## Consequences

- `AGENTS.md`, `docs/`, and anything else an agent wrote under the workspace
  directory go with it. A workspace under version control loses its working
  tree; the confirm names the path for this reason.
- A chat whose directory has already left the root keeps its rows: the
  delete refuses a directory the root does not list, so those records are
  removed one chat at a time.
