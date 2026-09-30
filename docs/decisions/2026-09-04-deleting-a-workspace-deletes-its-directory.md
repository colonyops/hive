# Deleting a workspace deletes its directory

- **Status:** accepted
- **Date:** 2026-09-04

## Context

The Agents area's delete ended a workspace's live terminals and dropped its
session records, and stopped there — the directory was the user's, possibly
under version control, so nothing touched it. The workspace list is a scan of
the root for directories holding an `agent-workspace.yaml`
(`agentws.Store.reloadLocked`), and the scan has no suppression: with the
manifest still on disk, the next refresh listed the workspace again. The row
vanished on delete and came back on reload (#354).

Delete and list disagreed about what a workspace is. Only two ways out: the
delete removes what the scan reads, or app state starts overruling the scan.

## Decision

Delete removes the directory. `AgentWorkspacesService.DeleteWorkspace` kills
every live terminal the workspace's sessions hold, deletes the directory and
everything under it, then removes the session records and reloads the store.

The order is what a failure leaves behind. Terminals go first because the
directory under their working directory is about to disappear. The directory
goes before the records, so a delete that cannot finish — a permission, an
unreachable root — still has the chat history to come back to, rather than
reporting a failure that already threw it away.

`agentws.RemoveWorkspace` is the only call in the package that removes files
it did not write, so it re-checks its target rather than trusting the caller:
`dir` must be a single path element local to the root, and it must name a
directory holding a manifest. The root itself, `.shared`, `mcps.yaml`, and a
symlink pointing outside all fail those checks. The service adds the same
listing guard the editor and reveal calls use — a directory the root does not
list as a workspace is refused, not deleted.

An archive flag was rejected. It keeps the files, but a tombstone is app state
deciding what a directory tree contains: the scan stops being the authority,
the flag has to travel (or fail to) between two machines sharing a synced
root, and a hidden workspace needs a restore surface to be honest. Renaming
the action to "Forget" was rejected for leaving no way to remove a workspace
at all.

Deleting a user's files needs the user's consent, so the confirm strip states
the full path it is about to delete and what goes with it, and the operation
has no undo — restoring is the user's own backup.

## Consequences

- Canvases, `docs/`, `AGENTS.md`, and anything else an agent wrote under the
  workspace go with the directory. A canvas outlives the chat that made it
  (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry),
  not the workspace it lives in.
- A workspace directory under version control loses its working tree. The
  confirm names the path for exactly this reason.
- A chat whose directory has already left the root keeps its rows in the
  sidebar: the delete refuses a directory the root does not list, so those
  records are removed one chat at a time. The sidebar offers no editor for
  such a row, so no path to the endpoint is lost.

## Reference

Related decisions: ADR workspace-directories-are-generated-and-disposable (the
authored/generated split — delete takes both), ADR
canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry
(canvases as files in the folder).
