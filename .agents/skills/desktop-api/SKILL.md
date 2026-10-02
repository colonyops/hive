---
name: desktop-api
description: Observe the Hive Desktop notification pipeline and force a re-poll through its loopback MCP server, instead of reading desktop-pipeline.db. Use to assert what an event produced in the inbox, force a refresh after a devserver overlay/action/scenario, discover the webhook port for a push, or confirm an actions.yml/flow edit actually loaded.
compatibility: Requires a running desktop instance built from this worktree. The loopback HTTP server (webhook listener + MCP server) is on by default; launch.env pins its port via HIVE_DESKTOP_HTTP_PORT. An MCP client, or curl and jq.
---

# Observe the pipeline through the MCP server

The desktop exposes its capabilities as MCP tools over `app.App` (ADR mcp-replaces-the-agent-facing-http-api), so
a test asserts against a supported surface instead of `desktop-pipeline.db`.
One loopback `http` server (on by default) serves webhook push (`/hooks/…`),
the MCP server (`/mcp`), and the liveness probe (`/api/status`) on one port.

Pair this with the **devserver** skill: devserver *acts* (rewrites GitHub,
pushes webhooks), these tools *observe and reload*.

## 1. Find the port and confirm the app is up

The HTTP port is written to the worktree's `launch.env`. Liveness stays a plain
GET, because a JSON-RPC handshake is the wrong shape for "is it running?":

```bash
API=127.0.0.1:$(grep HIVE_DESKTOP_HTTP_PORT launch.env | cut -d'"' -f2)
curl -s $API/api/status  | jq .   # {webhook:{running,host,port,pathPrefix}, api:{pathPrefix}}
curl -s $API/api/version | jq .   # revision + dirty flag of the running build
```

If `/api/status` refuses the connection, the app is not running, was not built
from this worktree, or the HTTP server is disabled (`http.enabled: false`). A
fresh `mise run desktop:dev:prepare` turns it on.

## 2. Connect

The endpoint is `http://$API/mcp`, Streamable HTTP, no authentication (loopback
bind, and no tool here spawns a process). **Prefer a real MCP client** — add it
once and the tools appear as ordinary tools:

```bash
claude mcp add --transport http hive-desktop "http://$API/mcp"
```

The server is stateless and answers `application/json` rather than SSE, so a
single `curl` also works when a client is not set up. Ask it what it serves —
the schemas are generated from the server's own Go types, so this never drifts:

```bash
mcp() { curl -s "http://$API/mcp" -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$1\",\"params\":${2:-\{\}}}"; }

mcp tools/list | jq -r '.result.tools[] | "\(.name)\t\(.title)"'
```

`tools/call` takes `{"name": …, "arguments": {…}}`, and a tool's answer is in
`.result.structuredContent`:

```bash
call() { mcp tools/call "{\"name\":\"$1\",\"arguments\":${2:-\{\}}}" \
  | jq '.result.structuredContent // .result'; }
```

## 3. The loop: reload → read → retry

There is **no server-side wait**. `refresh_sources` returns once the event log
is appended; the flow engine commits a moment later on its own goroutine. So
force the poll, then read and retry a few times:

```bash
call refresh_sources
for i in $(seq 1 10); do
  hit=$(call list_inbox '{"profile":"'"$P"'","detail":"full","limit":50}' \
    | jq --arg repo acme/widgets --argjson num 42 \
        '[.items[] | select(.payload.repo==$repo and .payload.num==$num)][0]')
  [ "$hit" != "null" ] && echo "$hit" | jq '{lifecycle, sourceState, unread, reason: .payload.reason}' && break
  sleep 1
done
```

Selecting on payload fields is what `detail: "full"` is for, and why this
narrows with `limit` — see [Read tools](#4-read-tools) before widening it.

A **webhook push** appends directly (no refresh needed) — push via devserver,
then read + retry the same way.

## 4. Read tools

```bash
call list_profiles                                    # ids to scope everything else by
call get_flow '{"profileId":"'"$P"'"}'                # the graph: node ids, types, wires
call list_inbox '{"profile":"'"$P"'"}'                # newest first; title/state/lifecycle, no payloads
call list_inbox '{"profile":"'"$P"'","feed":"'"$FEED"'"}'                  # presence == routed there
call list_inbox '{"profile":"'"$P"'","feed":"'"$FEED"'","archived":true}'
call list_inbox '{"externalId":"'"$EXT"'","detail":"full"}'  # a GitHub global node id, NOT repo#num
call list_feeds '{"profile":"'"$P"'"}'                # every declared feed, counts joined on
call list_inbox_item_events '{"itemId":'"$ID"'}'      # one item's history
call list_item_sessions '{"itemId":'"$ID"'}'          # hive sessions this item started
```

`get_flow` is where node ids come from — `execute_flow`'s `nodeId` and the
node-image tools take them, and nothing else on the surface reports one.

**Payloads are opt-in.** `list_inbox` and `list_inbox_item_events` default to
`detail: "summary"`, which omits the raw source payload — a PR body alone is
kilobytes and a listing repeats it per item, so a whole feed at `"full"` runs to
megabytes and will blow a tool-response limit. Narrow *first* (by `feed`, by
`externalId`, or with `limit`), then ask for `"full"`. Everything the app itself
names — `title`, `url`, `lifecycle`, `sourceState`, `unread`, `feedId`,
timestamps — is in the summary. Each answer echoes the `detail` it used, so an
omitted payload is never mistaken for an absent one.

A missing thing is `not_found`, never an empty collection: a profile id, a feed
id or an item id that resolves to nothing is an error, so an empty list always
means "nothing has landed here". `list_feeds` reports a declared feed at zero
rather than omitting it, and marks `declared: false` on a feed only stale inbox
rows still claim.

## 5. Test a flow without deploying it

`execute_flow` runs a flow against input you supply and returns what every node
did, committing nothing (ADR flows-are-dry-run-against-supplied-input). Reach for it *before* the deploy → refresh
→ read loop above: that loop writes feed membership, `kv` and notifications to
answer a question about a script, and it cannot tell a bug from "hasn't polled
yet".

```bash
call execute_flow '{
  "flowId": "triage", "nodeId": "src-github",
  "messages": [{"Key": "acme/app#1", "Payload": {"state": "open"}}]
}' | jq '.nodes[] | {nodeId, in, out, dropped, ok,
                     emitted: [.emitted[] | {port, keys: [.messages[].Key]}],
                     console: [.console[].text], error}'
```

- **Isolate one node.** `nodeId` is any node, not only a source — inject a
  captured payload straight at the `function` node under test and the source in
  front of it never runs.
- **Test an unsaved edit.** Send `flowYaml` (the file's text) or `flow` (the
  same document as a JSON object) instead of `flowId`.
- **Seed `kv`** (`{"<nodeId>": {"<key>": <value>}}`) to exercise notify-once
  against a known starting state. The real store is neither read nor written, so
  repeated calls give the same answer; what the run *would* have stored comes
  back in `.kvMutations`, and what it would have committed in `.outputs`.
- **Keep the response bounded** with `detail`. A payload is reported once per
  node it reaches, so a real one through a deep graph comes back many times
  over. `"emitted"` (the default) drops each node's `received` — it is the
  upstream's `emitted`; `"counts"` drops message bodies entirely and is what a
  snapshot run wants; `"full"` restores both.
- `console.log` in a function node is readable here and nowhere else — a live
  run discards it.
- A script error's `line` and the positions inside its `message` and `stack` are
  all relative to the `on_message` body, not to the wrapper the engine compiles.

## 6. Did my config edit load?

Both config surfaces report their own load status, so an edit is verified by
reading it back rather than by clicking through the UI. A file that fails to
parse leaves the previous version in effect, so `valid` is the only signal that
separates "accepted" from "rejected and ignored":

```bash
call list_actions  | jq '{path, valid, error, ids: [.actions[].id]}'
call list_profiles | jq '[.profiles[] | {id, valid}]'
```

`list_actions` returns each action's type-specific config too, so a template
edit is confirmed by the value that came back — `.actions[] |
select(.id=="…") | .clipboard.textTemplate`.

## Guardrails

- **A failing tool answers with a result, not an error.** Look for
  `.result.isError` and read `.result.content[].text`, which opens with a stable
  kind — `invalid`, `not_found`, `conflict`, `unavailable`, `internal`. Branch
  on that, never on the prose.
- **Match on the payload**, not the external id: `external_id` is the source's
  own id (a GitHub global node id), while the devserver and your test think in
  `repo`/`num`. Filter `.items[] | select(.payload.repo==… and .payload.num==…)`
  — which needs `detail: "full"`, so scope the read before you widen it. The
  same external id can exist across profiles/scopes, so by-id reads return a
  slice.
- **Reload only helps pull sources.** `refresh_sources` re-polls
  `sources.github`; it does nothing for webhook-fed feeds (those append on
  delivery). It answers `unavailable` in mock modes (no producer).
- **The tools need the HTTP server on.** They share that loopback server (on by
  default); with `http.enabled: false` there is nothing to mount on.
- **Reads, reloads and safe mutations only.** Session control is deliberately
  absent — starting a session is command execution and lives behind the
  token-guarded `/api/terminal/` prefix. Drive triage through the app or the
  devserver, and assert here. A dry run is part of that: it reports the outputs
  and KV writes a live run would have made instead of making them, so what it
  tells you about the commit is the graph's half. Two things the commit itself
  does are deliberately outside it — minting an inbox row for a key no source
  ingested, and dropping feed rows a snapshot no longer lists. Assert those
  through the inbox after a real refresh.
