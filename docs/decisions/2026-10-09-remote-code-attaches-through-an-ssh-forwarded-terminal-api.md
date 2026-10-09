# Remote Code attaches through an SSH-forwarded terminal API

- **Status:** accepted
- **Date:** 2026-10-09

## Context

Desktop's session database and tmux manager belong to one machine. The terminal
HTTP/WebSocket protocol already carries pane controls and output, but Wails
bootstraps only a local endpoint. We need a first remote connection without
moving files, credentials, or process inspection to the client.

## Decision

Remote Code connects to an explicitly selected loopback SSH-forwarded endpoint.
An authenticated session-list operation declares control and terminal wire
compatibility. The remote installation owns its engine and tmux processes;
the native client reuses the terminal renderer with a separate session list.
Local commands cannot target the remote view implicitly.

The `terminals` build tag enables tmux in a `server` build. Ordinary headless UI
tests keep their existing terminal-unavailable behavior. Popup PTYs remain
unavailable in server builds. Native notification services are omitted there.

An opt-in `HIVE_DESKTOP_CONNECTION_FILE` exports the per-run terminal token to
an atomically replaced mode-0600 file for retrieval through SSH. Tokens stay in
client memory, and SSH forwarding keeps the server's loopback boundary intact.

## Consequences

The first slice attaches to existing Code sessions. Creation, remote inbox and
Chats, file transfer, and automatic SSH provisioning are later work. Remote
views do not pass paths or PIDs into local services. A Docker fixture owns
persistent remote data and host keys and publishes only SSH on host loopback.
