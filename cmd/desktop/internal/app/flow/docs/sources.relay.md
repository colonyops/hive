# Hive Relay

A **Hive Relay** node reads one relay source. The relay is a hosted feed: it gives each source an ingress URL, stores every HTTP request sent to it exactly as delivered, and parses nothing. The desktop pulls those deliveries on the poll tick, so a service on the internet can reach a flow without the machine being reachable. The hosted relay is `relay.hivedesktop.com`; you can also run your own and connect to it. The node has no inputs; this is where a flow starts.

Its output is the relay's **window**. The relay keeps a source's deliveries for a limited time and up to a per-source count, and the node holds the items those deliveries parsed to. An item whose deliveries have aged out of the relay leaves the set; that makes it old, not resolved, and nothing archives it.

## Fields

- `credential` -- required. The connected relay account, written `relay/<host>-<email>`, e.g. `relay/relay.hivedesktop.com-you@example.com`.
- `source` -- required. The relay source id this node reads.
- `dialect` -- optional. How a delivery is parsed: `auto`, `generic`, `ntfy`, `gotify` or `healthchecks`. Empty is `auto`, which today parses everything as `generic`. `ntfy`, `gotify` and `healthchecks` arrive in a later release and are rejected until then.
- `map` -- optional. JSON Pointers (RFC 6901) that overlay the parsed item, applied after the dialect:
  - `key` -- the item's identity. It replaces the parser's key and becomes `id`.
  - `title`, `url`, `kind`, `state` -- the matching item fields.
  - `states` -- maps the value `state` resolved to (as a string) onto the state it means, e.g. `{"0": "down", "1": "resolved"}`. A value with no entry passes through.
- `checks` -- optional. Per-slug `period` and `grace` for the `healthchecks` dialect, which arrives in a later release.
- `interval` -- optional, e.g. `5m`. The shortest time between fetches. The relay still only loads on a poll tick, so the real cadence rounds up to the next one; empty fetches on every tick.
- `icon` -- optional glyph, from the curated feed icon set, shown on this source's items. Empty uses the default feed glyph.
- `image` -- optional uploaded image shown as this source's mark instead of the glyph. It is a content hash of a normalized PNG kept in the app data dir; it is presentation only and never affects ingest. Empty falls back to `icon`.

## What a delivery becomes

Under `generic`, one delivery is one item, by the same rule the local webhook node uses:

- When the body is a JSON object, its fields are the item, so `id`, `title`, `url`, `kind`, `state`, `repo` and `body` pass through.
- **Identity** is a top-level `id` (string or number). Without one it is the SHA-256 of the body, so an exact retry collapses onto the same item and any changed body is a new one.
- `title` falls back to `<path> · <first 12 characters of the key>`, with `/` as the path when the request had none. `url` is kept only when it is `http` or `https`.
- `kind` is `Webhook` unless the body sets one.
- A body that is not a JSON object (plain text, an array, a number) becomes `{"body": "<the text>"}`, with a hashed identity and the fallback title.
- Every item also carries `raw`, the stored request: `{"method", "path", "query", "headers", "body"}`. `query` and `headers` map a name to a list of values. `raw.body` is the decoded JSON value when the body is JSON, and the text otherwise.

Several deliveries with one key collapse to the newest. Items are emitted as `source_kind: relay`.

Encrypted deliveries are skipped for now; reading them arrives in a later release.

## Mapping

`map` is how a service that does not speak the generic contract still produces good items, because pointers reach into `raw.body`. Uptime Kuma's webhook, for example, sends one heartbeat per check:

```yaml
map:
  key: /raw/body/monitor/id
  title: /raw/body/monitor/name
  state: /raw/body/heartbeat/status
  states:
    "0": down
    "1": resolved
```

Each monitor is now one item that goes `down` when it fails and archives itself when it recovers. A pointer that resolves to nothing, or to an object or array, keeps the parser's value. `~1` stands for `/` and `~0` for `~` inside a key, and a number selects an array element.

## Failure

A tick reads every new delivery or fails. An unreachable relay, a rejected key, or any non-2xx answer fails the tick: nothing is emitted, the set and the read position stay as they were, and the next tick tries again. The failure is recorded in Activity.

A tick that finds no new deliveries emits the current set unchanged. A restart, a manual refresh, or reconnecting the account reads the relay's window again from the start.
