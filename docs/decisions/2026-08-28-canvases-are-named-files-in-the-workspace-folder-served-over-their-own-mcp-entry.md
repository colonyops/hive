# Canvases are named files, served over their own MCP entry

- **Status:** accepted
- **Date:** 2026-08-28

## Context

A workspace agent's only output channel was terminal scrollback, so anything
meant to be looked at had to be described rather than shown (#232). `mcpsrv`
already serves app tools over Streamable HTTP on the loopback server, and
`mcpcatalog`'s `RuntimeURL` resolves an app-hosted entry's address at render
time (ADR mcp-replaces-the-agent-facing-http-api).

## Decision

A **canvas** is a named document filled with **blocks** (`markdown`, `link`,
and `html`; see ADR
a-canvas-html-block-is-restricted-by-what-it-can-reach-not-by-how-it-looks).
Block ids are agent-chosen and upsert in place, so a block is revised rather
than duplicated. Where a canvas file lives is set by ADR
hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it.

**A canvas is owned by a space, not by a chat session.** The creating session
is recorded as provenance; it labels, never authorizes. Deleting a chat leaves
its canvases.

**Identity rides the launch environment.** A chat's launch sets
`HIVE_AGENT_SESSION=<record id>` and `HIVE_AGENT_WORKSPACE`
(`agent_workspaces_service.go`). Every canvas tool takes that id as its
`session` argument and the app derives the space from the record; the space is
never caller-supplied, and an unknown id is `not_found`.

**Canvas tools are their own catalogue entry, not more `hive-desktop` tools.**
Enabling an output surface must not require granting the app-control tool set,
and the reverse. Each app-hosted entry joins the loopback base with its own
`Descriptor.RuntimePath`, and a pinning test holds the catalogue's paths to the
adapter's mount constants. The server is now `hive-drive` at `/mcp/drive`;
`hive-canvas` at `/mcp/canvas` stays as the same server at its older address,
because workspace manifests name it.

Delivery to the pane is the native wake-up pattern: content is stored state, a
change publishes an event, and the pane re-reads.

## Consequences

- The canvas server keeps `mcpsrv`'s unauthenticated-behind-loopback posture.
  Any local process can claim any session id, as ADR
  mcp-replaces-the-agent-facing-http-api accepts for the whole surface; what
  such a write can do is put content in a pane the user reads.
- Removing the `hive-canvas` entry or the `/mcp/canvas` mount breaks existing
  workspace manifests.
