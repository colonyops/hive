# Function

A **function** node runs author-trusted JavaScript against every message that reaches it. It has 1 input and up to 16 outputs (`outputs`, default 1).

## Fields

- `on_message` (required) — the body of `function on_message(msg, node, state, kv) { ... }`. Return:
  - a single `msg` — goes out port 0
  - an array of `msg` — multiple messages, all on port 0 (when `outputs` is 1)
  - a port-indexed array (e.g. `[msg, null]`) — `array[i]` goes out port `i`, once `outputs` is more than 1
  - `null` — discard (reported, never silently dropped)
- `outputs` — 1 to 16, default 1.
- `timeout` — how long a single `on_message` call may run before it's terminated and the message is discarded as an error. 100ms to 60s, default 5s.

`on_message` is the whole lifecycle: there are no start or stop hooks. The node does no I/O, so a stop hook could only mutate state that is about to be discarded, and setup belongs inside `on_message` as lazy initialization:

```
state.counts ??= {};
```

## The msg shape

```
msg.Payload   // opaque — shape set by the source; reshape it toward the
              // canonical item contract (docs/decisions/0008) for feed rendering
msg.Key       // stable item identity (e.g. "colonyops/hive#2841")
msg.Topic     // "source:<source-id>"
msg.ID        // unique per log record
```

The message envelope is exactly these fields plus `Ts`, `SourceKind`, `SourceScope` and `OccurrenceKey`. Extra properties attached to `msg` itself are not carried to the next node — per-message data belongs in `msg.Payload`, which is opaque and passes through whole.

`msg.Payload` is usually a live object — never `JSON.parse` it. A scalar payload (a bare number or string) reads back `undefined` for any field access rather than throwing, so a change-detection recipe degrades to "never a meaningful change" instead of crashing.

## Durable state: `kv`

`kv` is a small durable key-value store scoped to this node — the memory behind "notify once" and "notify on change". Unlike `state` it survives restarts and redeploys, and a write becomes durable atomically with the tick that made it: a script that throws after `kv.set` persists nothing.

- `kv.get(key)` — the stored value, or `undefined`
- `kv.set(key, value, { ttl })` — store a JSON-serializable value; `ttl` is a whole number of seconds (omit or `0` = no expiry). An expired key reads as absent immediately — re-arming does not wait on cleanup
- `kv.has(key)` / `kv.delete(key)` / `kv.keys(prefix)` — prefix matching is case-sensitive

Values must be JSON-serializable (a function or `undefined` throws). Keys cap at 512 bytes and stored values at 4096 — this is a dedup memory, not a blob store.

Key on the full identity tuple, `JSON.stringify([msg.SourceKind, msg.SourceScope, msg.Key])`, not `msg.Key` alone: two sources feeding one node can emit the same external id for different items.

KV identity is the node **id**, which the editor preserves across renames — renaming a node keeps its memory and does not re-notify. The id disappearing is what reclaims it: deleting the node (or replacing it with a fresh one) clears its KV on the next deploy, converting the node to another type clears it too, while hand-recreating a node under the *same* id inherits the old memory. Keep dedup on the notify branch, not upstream of a feed — feeds recompute membership from full snapshots on deploy, with `kv` deliberately reading empty during that recompute, so a dedup in front of a feed and its live snapshot handling would disagree.

## Debugging: `console` and the dry run

`console.log` / `.info` / `.warn` / `.error` / `.debug` / `.trace` are available. Strings print verbatim, everything else as JSON. In a live run the output goes nowhere — it is a debugging affordance, not a log — so leaving a `console.log` in a deployed script costs nothing.

Where it *is* readable is a dry run: the `execute_flow` tool on this install's MCP server runs a flow against input you supply and returns what every node received, emitted per output port, and dropped, plus its console output and structured script errors with line and column. Nothing is committed — no feed membership, inbox rows, notifications, queued actions or durable `kv` — so it is safe to call repeatedly. The flow can be one that is installed or a document you have not saved yet; the input is delivered to any node you name, so a single function node can be exercised against a captured payload without its source running; and `kv` is an in-memory sandbox you seed, which is how notify-once logic is tested against a known starting state. The tool's own input schema is the request shape.

## Example

```
if (msg.Payload.state === "closed") return null;   // drop
msg.Payload.tag = "reviewed";
return msg;
```

## Splitting one message into many feed items

Return several messages, each with a `Key` you mint, to turn one source message
into one durable feed item per entity — the way to fan a metrics query with N
series into N items, each with its own payload, triage state, and actions:

```
return msg.Payload.result.map(function (s) {
  return {
    ...msg,                                  // keep Topic, SourceKind, SourceScope
    Key: [s.cluster, s.namespace, s.kind, s.name].join("/"),
    Payload: { title: s.kind + "/" + s.name, cluster: s.cluster, namespace: s.namespace },
    // No updatedAt: a metrics series carries no time of its own, so the item
    // is stamped when it is minted.
  };
});
```

Three rules make this work:

- **Mint `Key`, never `Topic`.** The key is the item's identity — set it to
  whatever makes each entity distinct. `Topic` is what scopes feed membership to
  its source; rewriting it detaches the item and breaks the lifecycle below.
- **Put what the item renders and acts on in `Payload`.** `title`, `url` and
  `updatedAt` are read from it; the rest is yours (an `applies_to` action reads
  `.Payload`). A key the source never emitted has no inbox row yet, so the feed
  mints one on first appearance from this payload.
- **Set `updatedAt` if the entity has a time of its own.** It is unix
  milliseconds and it is what a feed row shows as the item's age. Omit it and
  the item is stamped when it was minted, which is right for an entity that has
  no timestamp — a metrics series does not. Do not compute it from a payload
  field the source does not carry: `new Date(undefined)` is `0`, and the row
  then reads as decades old. The value is read once, at mint; a later poll
  re-claims the existing row without refreshing it.

Lifecycle is presence-based and automatic: each poll restates the whole set, so
an entity that stops appearing drops from the feed on the next poll (it moves to
Trash, and returns if the entity does — its read/unread state is kept). This
mirrors how a feed reconciles any source snapshot. One caveat: one item per
entity means an unbounded-cardinality query is an unbounded feed — key on a
bounded identity, not on an open-ended label.

## Behavior

Each node instance gets its own JavaScript VM, so a timeout only affects this node, never a sibling. `state` survives across messages for the lifetime of one Deploy, but is not durable across app restarts, and a node that times out is respawned with a fresh `state`.

A timeout interrupts the script cooperatively. Code that neither allocates nor returns to the interpreter — a tight empty loop — can outlive its interrupt; the node's message is still discarded as an error, and the abandoned evaluation consumes part of a fixed process-wide budget rather than blocking anything else.
