# Function

A **function** node runs author-trusted JavaScript against every message that reaches it. It has 1 input and up to 16 outputs (`outputs`, default 1).

## Fields

- `on_message` (required) — the body of `function on_message(msg, node, state) { ... }`. Return:
  - a single `msg` — goes out port 0
  - an array of `msg` — multiple messages, all on port 0 (when `outputs` is 1)
  - a port-indexed array (e.g. `[msg, null]`) — `array[i]` goes out port `i`, once `outputs` is more than 1
  - `null` — discard (reported, never silently dropped)
- `on_start` (optional) — runs once per instance before the first message; use it to initialize `state`.
- `on_stop` (optional) — runs once per instance on teardown (Deploy drain).
- `outputs` — 1 to 16, default 1.
- `timeout` — how long a single `on_message` call may run before it's terminated and the message is discarded as an error. 100ms to 60s, default 5s.

## The msg shape

```
msg.Payload   // opaque — shape set by the source (e.g. a PR/issue/notification)
msg.Key       // stable item identity (e.g. "colonyops/hive#2841")
msg.Topic     // "source:<source-id>"
msg.ID        // unique per log record
msg.Meta      // { source, kind, repo? } — kind is "pr" | "issue" | "notification" | ...
```

## Example

```
if (msg.Payload.state === "closed") return null;   // drop
msg.Payload.tag = "reviewed";
return msg;
```

## Behavior

Runs in a dedicated Web Worker per node instance (`isolate: true`) — a timeout only terminates this node, never a sibling. `state` survives across messages for the lifetime of one Deploy, but is not durable across app restarts.
