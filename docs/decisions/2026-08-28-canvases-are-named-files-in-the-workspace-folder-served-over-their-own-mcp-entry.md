# Canvases are named files in the workspace folder, served over their own MCP entry

- **Status:** accepted
- **Date:** 2026-08-28

## Context

A workspace agent's only output channel is terminal scrollback, so anything it
produces that is meant to be looked at has to be described rather than shown
(#232). The MCP surface the canvas needs already exists in shape: `mcpsrv`
serves app tools over Streamable HTTP on the loopback server, and
`mcpcatalog`'s `RuntimeURL` mechanism resolves an app-hosted entry's address
at render time (ADR mcp-replaces-the-agent-facing-http-api).

An earlier revision of this decision scoped one canvas to each chat session,
stored under the app's state directory and deleted with the session record.
Demo use showed that scope is wrong: a chat produces several distinct
artifacts, each worth addressing, keeping, and reopening after the chat is
gone — the session was the author, not the owner.

## Decision

A **canvas** is a named artifact in a workspace, filled with **blocks** —
`markdown` (rendered by the frontend's sanitizing GFM renderer; raw HTML is
escaped, never rendered) and `link` (http/https/mailto only) — and shown in a
pane in the Agents area. Block ids are agent-chosen and upsert-in-place, so a
block is revised rather than duplicated. No HTML block kind: rendering
agent-authored HTML in the webview is a security decision this deliberately
defers.

**A canvas is keyed by (workspace, name), not by chat session.** The name is
an agent-chosen slug (lowercase, filename-like; the store enforces it), the
canvas carries an optional display title, and a workspace holds as many as
its chats create. The creating session id is recorded as provenance — it
labels and default-picks, never authorizes. Deleting a chat or the workspace
record leaves canvases untouched; `delete_canvas` and the file system are how
one dies.

**Content lives in the workspace folder: `<workspace>/canvases/<name>.json`,
written atomically** (temp+rename), owned by `internal/app/canvas`. The
workspace folder is already the place where a chat's durable output lands
(authored `docs/`, the manifest), it survives workspace deletion by standing
rule, and it travels with the folder — so canvases are browsable in place,
under version control if the folder is, and shared across machines that share
the root. The generator's reconcile never touches `canvases/`: it prunes only
its own subtrees. JSON files keep the block model free to change without
migrations while it is young.

**Identity rides the launch environment.** The session record exists before
launch, so `launchTerminal` injects `HIVE_AGENT_SESSION=<record id>` (and
`HIVE_AGENT_WORKSPACE`) via `tmux new-session -e` — set on the session, so
windows the agent opens later inherit it too. Every canvas tool takes that id
as its `session` argument, resolves it against the session store, and derives
the workspace from the record; an unknown id is `not_found` and the workspace
is never caller-supplied. A session launched by a pre-canvas build lacks the
variable until relaunched, and the tool descriptions tell the agent to say so
rather than guess.

**A separate `hive-canvas` catalogue entry, not more `hive-desktop` tools.**
Enabling an output surface must not require granting the app-control tool
set, and vice versa. The entry is the second app-hosted server, which is what
made `RuntimeURL` path-aware: `Descriptor.RuntimePath` joins the loopback
base per entry (`/mcp`, `/mcp/canvas`), and a pinning test holds the
catalogue's paths to the adapter's mount constants.

Delivery to the pane is the native wake-up pattern: content is stored state,
`events.CanvasUpdated` → coalesced `canvas:updated` → the pane re-reads.
The frontend reads over the token-guarded agents HTTP client, addressed by
(workspace, name); it has no mutation surface at all — writes exist only as
MCP tools, so the pane can never race the agent.

## Consequences

- The threat model is unchanged: the canvas server keeps `mcpsrv`'s
  unauthenticated-behind-loopback posture and spawns nothing, but its writes
  now land inside the workspace folder — confined to `canvases/` by the
  store's name and workspace validation. Any local process can claim any
  session id — the same standing ADR mcp-replaces-the-agent-facing-http-api
  accepts for the whole surface; what such a write can do is put content in a
  pane the user reads.
- Canvases share the workspace folder's lifecycle, not the app's: they
  survive `DeleteSession` and `DeleteWorkspace` (which never touch the
  directory), appear in `git status` when the folder is a repository, and
  follow the folder to other machines.
- The pane's picker lists canvases by title (falling back to name) rather
  than by the chat that made them; `?canvas=<name>` on the agents route pins
  one, so a canvas is linkable.
- M1 is a viewport with agent read-back: the user cannot edit blocks. If
  user edits ever land, the read-back tools are where the agent would see
  them — the tool surface was shaped so that adding this does not change it.
