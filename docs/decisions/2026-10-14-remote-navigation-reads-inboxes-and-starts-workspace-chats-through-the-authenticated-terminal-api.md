# Remote navigation reads inboxes and starts workspace chats through the authenticated terminal API

- **Status:** accepted
- **Date:** 2026-10-14

## Context

The remote Code preview already authenticates an SSH-forwarded terminal API.
Workspace chat creation spawns processes on that same installation. Local and
remote installations can reuse profile, workspace, and session identifiers.

## Decision

Reuse the authenticated agent-workspace HTTP client for remote chat creation
and resumption. Add bounded, read-only inbox discovery and feed endpoints under
the same token-protected prefix. Keep remote catalogs, selections, and clients
in a separate frontend store. Navigation groups use the connection's color.
Remote rows never enter local stores or local filesystem actions.

## Consequences

One connected installation can supply Inbox, Code, and Chats navigation.
Changing or disconnecting the endpoint clears all remote selections and
invalidates pending results. Inbox triage, remote Code creation, Drive access,
and automatic SSH management remain outside this slice.
