# The desktop source pipeline

!!! warning "Experimental — partially implemented"
    The pipeline described here runs **alongside** the desktop app's original
    feed system today; it does not yet drive the sidebar. See
    [Current state and the deferred cutover](#current-state-and-the-deferred-cutover)
    for exactly what is and isn't wired up.

The source pipeline is a Node-RED-style graph editor and runtime built into
Hive Desktop: GitHub (and future RPC) sources feed an append-only event log,
a stateless frontend graph runtime processes it in small batches, and the
result is committed back to Go as persisted feed items and side-effecting
actions. It is reachable today via the command palette (`⌘K` → **"Open flows
editor…"**), independent of the sidebar's existing feed experience.

This document is the durable architecture reference for that system. It
covers the wire contract, the storage layer, the config schemas, the
frontend node-type contract, and what still needs to happen before the
pipeline can replace the legacy feed path.

## Overview and the tripartite model

The pipeline is split into three tiers that never share a database or a
process boundary:

1. **Go, write side (source producer)** — polls GitHub (today; RPC sources
   are schema-ready but not yet implemented) and appends whatever it finds to
   an append-only log, deduplicating by key so an unchanged item isn't
   re-appended every tick.
2. **TypeScript, stateless (frontend graph runtime)** — reads a page of log
   rows, runs them through one flow's graph (sources → filters/functions →
   feeds/actions) in a Web Worker (or an in-process fallback), and produces a
   single batch describing every output, discard, and per-node metric.
3. **Go, read/write side (commit + delivery)** — applies that batch
   atomically (upsert feed items, enqueue action invocations, record metrics,
   advance the read cursor) and serves the results back out: feed views read
   persisted feed items, and a separate output worker drains queued actions.

```
   GitHub API
       │
       │  feed.LiveProvider.SourceItems (cached, conditional, singleflight —
       │  the same fetch path the legacy feed system uses)
       ▼
 ┌───────────────────────┐
 │ pipeline.Producer      │  poll tick, dedupe unchanged payloads per key
 │  (source producer)     │
 └───────────┬───────────┘
             │ db.Append(topic="source:<id>", key, payload)
             ▼
 ┌────────────────────────────────────────────────────────────┐
 │ event_log  (desktop-pipeline.db — append-only, STRICT)      │
 └───────────┬──────────────────────────────────────────────┬─┘
             │ PipelineService.ReadFrom(offset, limit)       │
             ▼                                                │
 ┌────────────────────────────────────────────────────────┐   │
 │ Frontend graph runtime (stateless, per flow)             │   │
 │                                                          │   │
 │   github-source ──▶ github-filter ──▶ function ──┬──▶ feed     │
 │        (D1: passthrough)   (Web Worker)           └──▶ action   │
 │                                                          │   │
 │   runGraph() walks the DAG once per batch, producing a   │   │
 │   CommitResult: outputs / discards / node-run metrics    │   │
 └───────────┬──────────────────────────────────────────────┘   │
             │ PipelineService.Commit(CommitBatch)               │
             ▼                                                   │
 ┌────────────────────────────────────────────────────────────┐  │
 │ DB.CommitBatch — one SQLite transaction:                    │  │
 │   • upsert feed_item / enqueue output_command                │  │
 │   • insert node_run                                          │  │
 │   • advance consumer_offset  (idempotent: replay ≤ committed  │  │
 │     offset is a no-op)                                        │  │
 └───────────┬──────────────────────────────────────┬──────────┘  │
             │                                       │              │
             ▼                                       ▼              │
   FeedItems(feedID)                        pipeline.Worker.Tick()  │
   (flows editor's read-only                 drains output_command, │
   preview panel; the legacy                 auto_apply gate,       │
   sidebar does not read this yet)           dispatches Executors    │
                                                                     │
   consumer_offset  ◀──────────────── ConsumerOffset/Commit ────────┘
```

Delivery (`ReadFrom`/`Commit`/`FeedItems`) is exposed to the frontend by
`desktop/pipelineservice.go`; the append-only write side
(`internal/desktop/pipeline`'s `Producer`/`Source`) never talks to the
frontend directly.

## The `msg` contract

Every value flowing through the log and the graph runtime is a `Msg`,
defined once in `internal/desktop/pipeline/pipelinedb/log.go` and re-exported
verbatim as `pipeline.Msg` (a type alias, `internal/desktop/pipeline/source.go`):

```go
type Msg struct {
	ID      string
	Key     string
	Topic   string
	Ts      int64
	Payload json.RawMessage
	Meta    map[string]any
}
```

| Field     | Meaning                                                                 | Maps onto `event_log` as |
| --------- | ------------------------------------------------------------------------ | ------------------------ |
| `ID`      | Unique per log record — the engine's own commit cursor/dedup key         | `"offset"` (as a string) |
| `Key`     | The item's stable identity (e.g. `"colonyops/hive#2841"`), used for compaction and for `feed_item`/`output_command` dedup | `key` |
| `Topic`   | `"source:<source-id>"` — which configured source produced this message   | `topic` |
| `Ts`      | Unix nanoseconds when the row was appended                               | `created_at` |
| `Payload` | The opaque item JSON (shape is set by the source, e.g. a PR/issue/notification) | `payload` |
| `Meta`    | `{source, kind, repo}` set by the source producer on append; **not persisted** — `ReadFrom` always returns a `nil` `Meta`, this phase never stored it | *(no column — see below)* |

Two casing/location facts worth calling out explicitly, since they trip
people up:

- **`Msg` has no `json` struct tags at all**, unlike every other wire type in
  this codebase (`CommitBatch`, `Output`, `Sink`, …, all of which use
  lowerCamel JSON tags). That means `Msg` serializes under its literal Go
  field names, so a function node's `on_message` body reads `msg.Payload`,
  `msg.Key`, `msg.ID`, `msg.Topic`, `msg.Meta` — **capitalized**, not
  `msg.payload`/`msg.key`. See `desktop/frontend/src/pipeline/nodes/function/help.md`.
- `Meta` is a genuine gap, not a bug: `event_log` (see below) has no `meta`
  column, so anything a `Source` sets on `Msg.Meta` when it emits a message
  (e.g. `github_source.go` sets `{source, kind, repo}`) is visible to that
  one append call but is **not** persisted and **not** replayed — a message
  read back via `ReadFrom` always has `Meta == nil`. If a node's logic needs
  `kind`/`repo` durably, it must be encoded into `Payload` instead.

## The dedicated DB and table roles

The pipeline owns a separate SQLite database,
`internal/desktop/pipeline/pipelinedb` (`desktop-pipeline.db`, opened via
`pipelinedb.Open(desktop.StateDir(), …)`), deliberately isolated from hive's
shared `hive.db`. The package doc on `pipelinedb/db.go` states the reason
directly: desktop pipeline write traffic (a poll tick appending dozens of
rows, a commit batch running every pump) must never contend with the
CLI/TUI's own SQLite writer.

All five tables are declared `STRICT` (SQLite's opt-in type enforcement) in
`pipelinedb/migrations/0001_pipeline.up.sql`, with two later migrations
(`0002_output_command_key.up.sql`, `0003_output_command_retry.up.sql`) adding
the action-dedup key and bounded-retry columns to `output_command`:

| Table | Role |
| --- | --- |
| `event_log` | Append-only log of every message a source has ever produced. `"offset"` (quoted — a SQLite keyword) is the autoincrement primary key and the thing everything else replays from. Indexed on `(topic, "offset")`. |
| `consumer_offset` | One row per consumer (a flow id), tracking the last offset that consumer's commit has fully accounted for. |
| `feed_item` | Go-owned, persisted output of a flow's `feed` nodes — primary key `(feed_id, item_id)`, so a re-commit of the same item is an upsert, not a duplicate row. |
| `output_command` | The queue of side-effecting action invocations waiting for the output worker, deduped on `(action_id, key)` (unique index `idx_output_command_action_key`), with `status`/`attempts`/`last_error` for bounded retry. |
| `node_run` | Per-node, per-tick execution metrics (`in_count`/`out_count`/`drop_count`/`ok`/`err`/`dur_ms`) for the flows editor's debug panel and RECENT list. |

Both `event_log`/`consumer_offset` and `feed_item`/`output_command`/`node_run`
share the same migration runner, `internal/data/migrate` — a small,
storage-agnostic package (`Load`/`Apply`/`Up`/`Down` over an `fs.FS` of
`NNNN_name.{up,down}.sql` pairs, tracked in a `schema_migrations` table) also
used by hive's own `hive.db`. `pipelinedb.Open` calls `migrate.Up` directly
with no legacy-bootstrap step, since this database has no pre-migration
history to reconcile.

## The log API

`pipelinedb.DB` (`log.go`) exposes the whole event-log surface:

- **`Append(ctx, topic, key, payload) (offset int64, err error)`** — inserts
  one row, stamping `created_at` as `time.Now().UnixNano()`.
- **`ReadFrom(ctx, offset, limit) ([]Msg, nextOffset int64, err error)`** —
  rows with `"offset" > offset`, ascending, up to `limit`. If nothing
  matches, `nextOffset` is the input `offset` unchanged, so a caller can
  always resume with `ReadFrom(ctx, nextOffset, limit)`.
- **`ConsumerOffset(ctx, consumer) (int64, error)`** / **`Commit(ctx,
  consumer, offset) error`** — read/write a consumer's checkpoint directly.
  `Commit` is monotonic in SQL (see the commit-protocol section below), so an
  out-of-order or replayed call never regresses a consumer's checkpoint.
- **`Compact(ctx) error`** — reclaims space in three independent, order-safe
  passes, using `pipelinedb.DefaultCompactOptions()` (30 days / 100k rows)
  unless a caller supplies its own `CompactOptions` at `Open`:
    1. **Key-compaction** — for every non-empty `key`, keep only the
       highest-offset row (log-compaction proper — the table's namesake
       behavior). Empty-key messages are exempt (they have no identity to
       compact against).
    2. **Age retention** — drop rows older than `MaxAge` (skipped if zero).
    3. **Count retention** — if still over `MaxRows`, drop the oldest rows
       until the cap is met (skipped if zero).

   Compaction is always safe to run: consumers resume from their own
   committed offset, so it needs no coordination with in-flight readers.

`Producer` (below) additionally keeps its own **in-memory**, non-persisted
`seen` map (topic+key → last payload) so an unchanged item isn't
re-`Append`ed on every poll tick even before compaction ever runs — a soft
optimization a restart forgets, not a substitute for `Compact`.

## The commit and offset protocol

`pipelinedb/commit.go` defines the wire shape the frontend graph runtime
commits back to Go, re-exported as `pipeline.CommitBatch` and friends
(`pipeline/commit.go`'s alias block):

```go
type Sink struct {
	Kind     string `json:"kind"`     // SinkKindFeed | SinkKindAction
	TargetID string `json:"targetId"` // feed id, or action id
}

type Output struct {
	Sink    Sink
	Key     string          // feed_item.item_id, and the output_command dedup key
	Payload json.RawMessage
	Unread  bool            // feed items only
}

type Discard struct {
	MsgID  string
	NodeID string
}

type CommitBatch struct {
	Consumer   string        // event_log consumer key — the flow id
	UpToOffset int64         // advance consumer_offset to here
	Outputs    []Output
	Discards   []Discard
	NodeRuns   []NodeRunView
}
```

`Sink.Kind` is the terminal tag a flow's two terminal node types stamp:
`feed` nodes commit `Sink{Kind: SinkKindFeed, TargetID: <feed id>}` (upserted
into `feed_item`); `action` nodes commit `Sink{Kind: SinkKindAction,
TargetID: <action id>}` (enqueued into `output_command`, deduped on
`(action_id, key)` via `ON CONFLICT (action_id, key) DO NOTHING`). Each
terminal node type's own `config.ts` (`nodes/feed/config.ts`,
`nodes/action/config.ts`) is the single source of truth for its `sink()`
function — `runGraph.ts` calls it rather than re-encoding the mapping.

`DB.CommitBatch` (`pipelinedb/commit.go`) applies one `CommitBatch` inside a
single SQLite transaction:

1. Read the consumer's current `consumer_offset`.
2. **Idempotency by offset**: if `UpToOffset` is at or below the currently
   committed offset, the whole call is a no-op — this batch was already
   applied by a previous commit (or is a stale/out-of-order retry) — nothing
   is written, not even `node_run`.
3. Otherwise: upsert every feed `Output` into `feed_item`, enqueue every
   action `Output` into `output_command`, insert every `NodeRunView` into
   `node_run`, and advance `consumer_offset` to `UpToOffset` — all in the
   same transaction as an `INSERT … ON CONFLICT … WHERE excluded."offset" >
   consumer_offset."offset"` upsert, so the offset itself can never regress
   even if two commits race.

Note what `CommitBatch` does *not* persist: `Discards` are accepted purely
for a caller that wants to log/count them (the corresponding node's
`node_run.drop_count` is the durable record) — no `discard` table exists.

**The "fully accounted for" invariant.** The frontend engine (`runGraph.ts`)
is built so that advancing the offset is always safe: every message in the
input batch ends up as exactly one of

- a **terminal output** (reached a `feed`/`action` node — becomes an
  `Output`),
- a **discard** (an unrouted message, a node that returned `null`, an
  unwired output port, or a `disabled: true` node), or
- an **errored discard** (the node's `onMsg` threw, or the transport timed
  out).

This isn't a runtime check that skips ahead — `runGraph` structurally
guarantees it (see [The node-type contract](#the-node-type-contract-frontend-d2)
below), and `computeUpToOffset` simply takes the max `ID` across the whole
input batch. Because that invariant holds, committing `UpToOffset` past a
message is always correct: nothing that message could still be "in flight"
for is left unaccounted.

## The three config files and schemas

### `flows/*.yaml`

Implemented by `internal/desktop/pipeline/flow`
(`internal/desktop/desktop.FlowsDir()`, default
`$XDG_CONFIG_HOME/hive/desktop/flows/`, override `HIVE_DESKTOP_FLOWS`). A
flow's **id is its filename stem** (`triage.yaml` → id `triage`) — never a
value read from inside the file, so the file and its id can never disagree
(`flowIDFromFilename`).

Top-level shape:

```yaml
version: 1              # required, must be 1
name: Frontend Triage    # optional display name
enabled: true            # optional, defaults to true (a pointer field distinguishes "absent" from "false")
nodes: [ ... ]
wires: [ ... ]
```

Each node carries the common envelope (`id`, `type`, `name?`, `disabled?`)
plus per-type fields inline, decoded by a **two-pass strict decode**
(`flow/node.go`'s `UnmarshalYAML`, mirrored for JSON in `node_json.go` for
the Wails wire shape): first a lax decode reads just the `type:`
discriminator, then the type is looked up in a registry
(`flow/node.go`'s `registry` map) and the remaining fields — with the
reserved envelope keys stripped — are strictly re-decoded (`KnownFields(true)`)
into a fresh per-type config, so both an unknown node `type` and an unknown
per-type field are hard errors.

| `type` | In / Out | Config fields | Notes |
| --- | --- | --- | --- |
| `github-source` | 0 / 1 | `source` (a `github-*` id in `profiles/*.yml`) | Backend-run — see [source producer](#source-producer-and-output-worker). |
| `rpc-source` | 0 / 1 | `source` (a `rpc` id in `profiles/*.yml`) | Schema exists; no producer or frontend node type implements it yet. |
| `github-filter` | 1 / 2 | `repos`/`exclude_repos`/`authors`/`exclude_authors`/`labels`/`exclude_labels` (doublestar globs), `types` (`pr`\|`issue`), `reasons` (GitHub notification reasons) | Port 0 = pass, port 1 = fail. At least one filter group must be set (a hard error if all are empty). |
| `function` | 1 / 1..16 (`outputs`) | `on_message` (required), `on_start?`, `on_stop?`, `outputs?` (1–16, default 1), `timeout?` (100ms–60s, default 5s) | Author-trusted JS, no sandbox. |
| `feed` | 1 / 0 (terminal) | `feed` (a feed id in `profiles/*.yml`) | Commits `Sink{feed, <feed id>}`; new items land unread. |
| `action` | 1 / 0 (terminal) | `action` (an id in `actions.yml`) | Commits `Sink{action, <action id>}`. |

Wires are directed edges: `{from, out?, to}`, `out` defaulting to `0`.
Validation (`flow/validate.go`'s `validateFlow`) runs, in order:

- **Hard errors** (any one fails the whole flow): missing/invalid/duplicate
  node ids (`^[a-z0-9][a-z0-9-]*$`, max 64 chars — `flow/slug.go`); a node's
  own `Validate(refs)` failing (a required field missing, an invalid glob,
  an unresolved `source`/`feed`/`action` reference, an out-of-range
  `outputs`/`timeout`); a wire naming an unknown node; a wire sourced from a
  terminal (0-output) node or targeting a source (0-input) node; a wire's
  `out` port out of range for its source node's declared output count; a
  duplicate wire; and a cycle (DFS-based, `flow/validate.go`'s `findCycle`,
  reporting the cyclic path).
- **Soft warnings** (only computed once every hard check passes): a
  `disabled: true` node; a terminal node with no inbound wire; the flow
  having no terminal node at all.

Cross-file references (does a `source`/`feed`/`action` id exist, and what
kind is it) are resolved through an injected `flow.Refs` interface — the
package never imports the profiles or actions loaders itself, so those can
be wired in independently (`desktop/flowsrefs.go`'s `flowRefsAdapter` is the
real implementation; `flow.MapRefs` is the map-backed test double used
throughout `flow`'s own test suite).

Each flow file has a machine-written sibling layout file, `<id>.ui.yaml`
(`flow/layout.go`): node canvas positions only, keyed by node id. It is
purely cosmetic — never consulted by `LoadFlow`/validation, and a missing or
broken layout file is not an error (the editor just lays nodes out fresh).
`LoadFlows` explicitly skips `*.ui.yaml`/`*.ui.yml` files when scanning the
flows directory for flow definitions.

**Worked example** (also `flow.ExampleFlow()`, see
[Starter-flow example](#starter-flow-example) below; a similar fixture — but
with `msg.payload` written lowercase, since it's a pure YAML round-trip test
that never actually executes the JS — lives in `flow/loader_test.go`'s
`workedExampleYAML`):

```yaml
version: 1
name: Frontend Triage
nodes:
  - { id: in-prs, type: github-source, source: team-prs }
  - { id: drop-bots, type: github-filter, exclude_authors: ["*[bot]"], repos: ["colonyops/*"] }
  - id: tag
    type: function
    outputs: 2
    on_message: |
      if (msg.Payload.state === "closed") return null;
      msg.Payload.tag = "review"; return [msg, null];
  - { id: team-feed, type: feed, feed: team-review }
  - { id: spawn-review, type: action, action: review-pr }
wires:
  - { from: in-prs, to: drop-bots }
  - { from: drop-bots, to: tag }
  - { from: tag, out: 0, to: team-feed }
  - { from: tag, out: 0, to: spawn-review }
```

### `profiles/*.yml` (current state)

Implemented by `internal/desktop/feed` (`desktop.ConfigPath()`, default
`$XDG_CONFIG_HOME/hive/desktop/profiles.yaml`). This is the **pre-existing**
schema the legacy feed system reads — `sources:` (GitHub search/notifications
queries) and `profiles:` → `feeds:` (client-side filtered views over one or
more sources), with each feed's own `filters:` block
(`internal/desktop/feed/filters.go`'s `FilterDef`).

The flow schema's `github-source`/`rpc-source`/`feed` nodes are an
**additive** consumer of this same file — a `github-source` node's `source:`
field and a `feed` node's `feed:` field both resolve against
`profiles/*.yml` via `flow.Refs` (`desktop/flowsrefs.go`). Nothing about
`profiles/*.yml` changes to support flows; the legacy feed filters
(`FilterDef`, the sidebar's own filtering) keep working exactly as they do
today, independent of whether any flow references the same sources/feeds. A
future cutover (see [below](#current-state-and-the-deferred-cutover)) would
retire `FilterDef`-based filtering in favor of `github-filter` flow nodes,
but that hasn't happened — both mechanisms are live simultaneously right
now, and a source or feed id can be referenced by a flow node with no
awareness on the profiles side that it's being read that way.

### `actions.yml`

Implemented by `internal/desktop/pipeline/actions`, at
`desktop.ActionsPath()` — **`$XDG_CONFIG_HOME/hive/desktop/actions.yml`**,
next to `profiles.yaml`, *not* a repo-scoped `.hive/actions.yml`. The design
doc calls this file `.hive/actions.yml`, but the desktop app's config is
global rather than tied to any one repo, so `ActionsPath()`'s doc comment is
explicit that it deliberately lives beside the desktop's other config
instead. (`EnvActionsPath` — `HIVE_DESKTOP_ACTIONS` — overrides the location
outright.)

The package mirrors `flow`'s own registry + two-pass strict decode:

```yaml
version: 1
actions:
  - id: review-pr
    label: Spawn review agent
    type: launch-session       # | shell | publish-event
    applies_to: [pr]           # optional; restricts a future detail-pane picker to msg.meta.kind values
    auto_apply: false          # default false — see below
    prompt_template: "Review {{ .Payload.title }}"
    agent: claude              # optional
```

| `type` | Config fields | Executor |
| --- | --- | --- |
| `launch-session` | `prompt_template` (required, Go template), `agent?`, `repo_template?`, `post?` | `LaunchSessionExecutor` — **currently a logging stub** (`LoggingSessionLauncher`); real session-spawn wiring is deferred, since the desktop app deliberately excludes `internal/hive`'s session/core machinery today. |
| `shell` | `command_template` (required), `cwd?`, `timeout?`, `env?` | `ShellExecutor` — runs `sh -c <rendered command>` for real; author-trusted, no sandboxing beyond cwd/env/timeout. |
| `publish-event` | `topic` (required) | `PublishEventExecutor` — **currently a logging stub** (`LoggingEventPublisher`); no real event bus is wired in yet (see the package doc on `publish_event_executor.go` for why `internal/core/eventbus`/`internal/core/messaging` are both poor fits). |

`id`/`label` are required envelope fields; `id` follows the same slug rule
as flow node ids. `AutoApply` gates whether `pipeline.Worker` (the output
worker) executes a queued `output_command` automatically at all — see
below.

`actions.ActionStore` (`actions/store.go`) is the same last-good-on-failure
posture as `flow.FlowStore`/`feed.Store`: a broken `actions.yml` on reload
keeps serving whatever last parsed cleanly, rather than blanking every
action out from under a running flow.

## The node-type contract (frontend, D2)

Every node type lives under
`desktop/frontend/src/pipeline/nodes/<type>/`, one directory per type
(`action`, `feed`, `function`, `github-filter`, `github-source` today —
`rpc-source` has no frontend directory yet, only a backend `flow` schema
entry):

| File | Role |
| --- | --- |
| `config.ts` | **Single source of truth** for the type's `Config` shape, its palette metadata (`label`/`category`/`glyph`/`role`/`defaults`), pure helpers, and `validate()` (a UX-only mirror of Go's authoritative `SaveFlow` validation, for live drawer feedback). |
| `runtime.ts` | The worker-side `ProcessorRuntime` (`onMsg`, plus optional `start`/`stop`) — only for `role: 'processor'` types. Must never import Vue or any DOM global (enforced by `__tests__/import-hygiene.spec.ts`), since it runs inside a Web Worker in production. |
| `editor.vue` | The drawer body Vue component: `props: {config, errors?}`, `emits: update:config` — a controlled component. |
| `help.md` | Rendered in the drawer via `lib/markdown.ts`. |
| `index.ts` | Wires the above into one `NodeTypeDefinition` via `defineNodeType()` (`nodeType.ts`). |

Two separate registries, both built via Vite's `import.meta.glob`
(`registry.ts`):

- **Worker registry** (`processorRegistry`) — over every `runtime.ts`, keyed
  by `type`. What actually executes a message.
- **App registry** (`byType` / `palette`) — over every `index.ts`, keyed by
  `type`. Palette entries, `instantiate()` for a fresh node, drawer editors.

`role` is the discriminant that decides *where* a node type runs:

- `'source'` — backend-run (`github-source`/`rpc-source`). No `runtime.ts`
  at all; the frontend only relays whatever the backend producer already
  appended, filtering entry-node messages by `config.source`'s log topic
  (`"source:<id>"`).
- `'processor'` — a Web Worker (`github-filter`, `function`).
- `'output'` — an engine-collected commit intent, never actually "run" — a
  terminal node's own `sink()`/`unread` in its `config.ts` tells `runGraph`
  how to tag a `CommitBatch.Output` (`feed`, `action`).

**Transport.** `runGraph` never touches a real `Worker` directly — it drives
every processor node through an injected `WorkerTransport`
(`engine/transport.ts`):

- `InProcessTransport` runs a `ProcessorRuntime` directly on the calling
  thread, wrapped in a `Promise`, and enforces `timeoutMs` itself by racing
  the promise against a deadline. This is both the unit-test double and
  **the actual production default today** — `driver.ts`'s
  `PipelineDriverOptions.transport` falls back to it, and
  `FlowsView.vue`/`usePipelineRuntime.ts` never construct anything else, so
  despite `WebWorkerTransport` existing and being fully unit-tested, no code
  path in this repo currently instantiates one.
- `WebWorkerTransport` (`engine/webWorkerTransport.ts`) is the intended
  production transport: one **shared** worker hosts every `isolate: false`
  runtime (`github-filter` today); a **dedicated** worker is spawned per
  `isolate: true` node *instance* (`function`), so a timeout's `terminate()`
  kills only that one instance, never a sibling or the shared worker. It
  owns only the message-protocol glue (request/response correlation, state
  merged back across the `postMessage` structured-clone boundary) — wiring
  it to a real bundled worker script is left to whoever does this
  production hookup; nothing here has done it yet.
- On a timeout (`NodeTimeoutError`, distinguished from an ordinary thrown
  error) `runGraph` calls `transport.reset(instanceId)` — "terminate,
  respawn" — so the next run starts clean; an ordinary node error needs no
  reset, since the node returned control fine.

**Engine execution** (`engine/runGraph.ts`), per batch:

1. Topologically sort the flow (`engine/graph.ts`'s Kahn's-algorithm
   `topoSort`; a cycle here is defended against, but Go's `SaveFlow`
   validation should already have rejected it).
2. Seed every in-degree-0 ("entry") node from the input batch — a
   `github-source`/`rpc-source` node only accepts messages on its own
   `"source:<id>"` topic; a bare entry node with no `source` field accepts
   everything (useful for tests exercising one node in isolation). Anything
   matching zero entry nodes becomes an immediate discard
   (`UNROUTED_NODE_ID`).
3. Walk nodes in topological order, per pending message: `disabled` nodes
   discard; `github-source`/`rpc-source` pass straight through to their
   wires; terminal (`feed`/`action`) nodes become an `Output`; everything
   else runs through the transport.
4. **Deep-clone fan-out**: forwarding one message to more than one wire
   `structuredClone`s it per destination, so one downstream branch mutating
   `msg.Payload` can never affect a sibling branch (or this node's own
   now-stale reference) — a single wire needs no clone.
5. **Unwired-port → discard**: a node produces output on a port with zero
   outgoing wires, that message is discarded (accounted for, not silently
   dropped) — this is exactly how `github-filter`'s "drop on fail" behavior
   works today (port 1 left unwired).
6. A `function` node's per-instance `state` (keyed `"<flowId>:<nodeId>"`)
   is held in a `Map` owned by the caller (`driver.ts`'s `PipelineDriver`),
   so it survives across `pump()` calls for the life of one running flow —
   not durable, forgotten on an app restart, matching the "stateless
   frontend" design posture (only this in-memory object, never anything Go
   persists).

## Deploy and drain semantics

`FlowsService.SaveFlow` (`desktop/flowsservice.go`) delegates to
`flow.FlowStore.Save`, which **re-runs the same `validateFlow` checks
`LoadFlow` does** before writing anything — an invalid flow is rejected
outright, so neither the on-disk file nor the store's in-memory snapshot
ever regresses to a broken state. The actual YAML write,
`flow.SaveFlow` (`flow/save.go`), is comment-preserving: a brand-new file
gets a short header
(`"# Hive Desktop flow — nodes and wires, as code. …"`) plus a clean
marshal; an existing file has its `yaml.Node` tree edited in place —
`version`/`name`/`enabled` set as scalars, `nodes`/`wires` sequences
replaced wholesale — so the document's header and any comments on unrelated
top-level keys survive, though comments attached to a specific node/wire
entry do not (there's no way to tell, from a `Flow` value alone, which
individual entry actually changed).

A `flow.FlowsWatcher` (`flow/watcher.go`, `fsnotify` on the flows
*directory*, 250ms debounce) fires on any `*.yaml`/`*.yml` change —
including the app's own `SaveFlow`/`SaveLayout` writes, and a `.ui.yaml`
layout-only edit — reloading `FlowStore` and emitting the Wails
`"flows:updated"` event so the frontend picker/list stays current
(`FlowsView.vue`'s `Events.On('flows:updated', …)`).

"Drain in-flight, then swap" (the design's stated Deploy semantics, and
`flow/save.go`'s own header comment) is realized more by construction than
by an explicit drain step today: `usePipelineEditor.ts`'s `addNode`/
`updateNode`/`deleteNode`/`deploy` all mutate the **same in-memory `Flow`
object** the active `PipelineDriver` already holds a reference to
(`FlowsView.vue` only builds a fresh `usePipelineRuntime` — and fresh
`PipelineDriver` — when the *selected flow's id* changes, not on every
edit or on Deploy of the same flow). So an edit is visible to the very next
`pump()` without any explicit reload, and `usePipelineRuntime.ts`'s
`pumping` guard prevents two overlapping `pump()` calls from racing each
other over the same cursor — that overlap-guard is the practical
"drain before the next run starts" behavior today, rather than a
Deploy-triggered wait for an in-flight run to finish.

## Source producer and output worker

**`pipeline.Producer`** (`internal/desktop/pipeline/producer.go`) is the
poll loop: each tick, it resolves the current `SourceLister` (re-read every
tick, so a source added/removed in `profiles/*.yml` takes effect without a
restart), calls `Produce` on each `Source`, and appends every emitted `Msg`.
The only implementation today, `githubSource`
(`internal/desktop/pipeline/github_source.go`), doesn't fetch GitHub
itself — it delegates to `feed.LiveProvider.SourceItems`, the exact same
cached/conditional/singleflight fetch path (`internal/desktop/feed/live.go`)
the legacy feed system already uses, so the pipeline gains a producer
without a second GitHub-fetching implementation to keep in sync. A source
fetch failure is logged and skipped; it never blocks other sources in the
same tick. `NewGithubSourceLister` builds one `githubSource` per configured
`feed.SourceDef`.

**`pipeline.Worker`** (`output_worker.go`) is the output side: on each tick
it drains up to `DefaultOutputWorkerBatch` (50) pending `output_command`
rows, resolves each one's `actions.Action`, and — only if that action's
`AutoApply` is `true` — renders its templates and dispatches to the
registered `Executor` for its type via a `Dispatcher`. `AutoApply: false`
(the `actions.yml` default) leaves a command sitting `pending`
indefinitely; there is no separate "awaiting confirmation" status — the
worker just re-checks `AutoApply` on every tick, so flipping it to `true`
picks up already-queued commands on the very next tick, with **no manual
confirmation UI built yet** to fire a non-auto-apply command by hand. A
failed execution is retried (with `last_error` recorded) until
`MaxOutputCommandAttempts` (5), then marked permanently `failed`; an unknown
`action_id` (e.g. `actions.yml` was edited to remove it) is marked failed
immediately, no retries.

## Testing strategy

- **Go unit tests** — every package under `internal/desktop/pipeline/...`
  has its own `_test.go` files exercising real SQLite via `t.TempDir()`
  (`pipelinedb`'s commit/log/feed-item/output-command/node-run tests),
  the `flow`/`actions` packages' strict-decode/validate/save/load round
  trips, and the executors/producer/output-worker business logic with fakes
  for `ActionLister`/`OutputCommandStore`/`Appender` where a real DB isn't
  needed. Run with `mise run test` (or targeted:
  `go test ./internal/desktop/pipeline/...`).
- **Vitest** — `desktop/frontend/src/pipeline/**/__tests__` covers the
  engine (`graph`/`runGraph`/`transport`/`webWorkerTransport`), the
  registries (including a `registry-parity.spec.ts` and an
  `import-hygiene.spec.ts` that enforces `runtime.ts` never imports Vue/DOM),
  each node type's `config.ts`/`editor.vue`, the composables
  (`usePipelineEditor`/`usePipelineRuntime`), and `driver.ts`. Run with
  `cd desktop/frontend && npm test`.
- **Playwright e2e** (`desktop/e2e`) — drives a real server build (`mise run
  desktop:e2e`, or `desktop:serve` for interactive use) in
  `HIVE_DESKTOP_MOCK=feed`/`onboarding` mode against `127.0.0.1:8931`
  (`8080` is commonly occupied by an unrelated local process, hence the
  non-default port). `desktop/e2e/tests/flows-editor.spec.ts` is additive
  DOM/text coverage of the flows editor surface (opening it from the
  command palette, the empty state, the node palette). Deeper coverage —
  a stateless-commit/replay spec, and screenshot snapshots for the flows
  canvas — is deferred future work; see the next section.

## Current state and the deferred cutover

As of this phase, the source pipeline is **additive**: it runs fully
alongside the legacy feed system, not in place of it.

- The flows editor is reachable via `⌘K` → **"Open flows editor…"**
  (`App.vue`'s `mode` ref swaps the whole main area between `'feed'` and
  `'flows'`), but the **sidebar still reads exclusively from the legacy feed
  path** — `internal/desktop/feed.Store`/`useFeedState.ts` — not from
  `feed_item`. A `feed` flow node's committed items are visible today only
  through the flows editor's own read-only preview panel
  (`FeedItemsPreview.vue`, backed by `PipelineService.FeedItems`).
- The legacy feed system's own orchestration — `internal/desktop/feed`'s
  `FilterDef` (`filters.go`) and `LiveProvider.fetchSourceDirect`
  (`live.go`) — is untouched and fully in charge of what the sidebar shows.
  The pipeline's `githubSource` producer *reuses*
  `LiveProvider.SourceItems` (the cached layer above `fetchSourceDirect`)
  rather than re-fetching GitHub itself, but that's read-sharing, not a
  dependency in the other direction — deleting `FilterDef`/
  `fetchSourceDirect` today would break the legacy feed path outright.
- No flow runs unless a user has hand-authored one and clicked **Deploy** —
  there is no "default flow" that starts automatically, and nothing writes
  `flow.ExampleFlow()`'s content to disk at startup (see below).
- `rpc-source` exists as a `flow` schema node type with backend validation,
  but has no source producer implementation and no frontend node-type
  directory (`nodes/rpc-source/` doesn't exist) — it can be referenced in a
  hand-written `flows/*.yaml`, but nothing runs it yet.
- `WebWorkerTransport` is fully implemented and unit-tested but not wired
  into the running app — `InProcessTransport` (main-thread) is what actually
  executes every processor node today.
- `launch-session` and `publish-event` actions execute against **logging
  stubs** (`LoggingSessionLauncher`, `LoggingEventPublisher`) — only `shell`
  actually does anything outside a log line.

**The deferred cutover** — tracked as separate follow-up work, deliberately
out of scope here because it is a large, Docker-e2e-heavy switchover that
would break the running app if done incompletely and can't be fully
validated outside that environment — is:

1. A default flow that runs automatically (so the pipeline actually
   replaces, not just parallels, today's polling), likely seeded from
   something like `flow.ExampleFlow()`'s shape but with real resolvable
   `source`/`feed` ids rather than placeholders.
2. Switching the sidebar (`useFeedState.ts` and its feed components) to
   read `feed_item` via `PipelineService.FeedItems` instead of
   `internal/desktop/feed.Store`.
3. Only once (1) and (2) are live and verified: deleting
   `internal/desktop/feed`'s `FilterDef`-based orchestration and
   `fetchSourceDirect`'s direct callers, since at that point the flows
   pipeline is the only thing driving the sidebar.
4. Rewriting/adding e2e screenshot snapshots and a stateless-commit/replay
   spec, all of which need the Docker-based `mise run desktop:e2e` gate to
   verify against real rendered output.

## Starter-flow example

`flow.ExampleFlow()` (`internal/desktop/pipeline/flow/example.go`) mirrors
`feed.ExampleConfig()`'s role: a concrete, commented, worked
`flows/*.yaml` document — the same github-source → github-filter →
function(outputs: 2) → {feed, action} pipeline described above — for docs
and any future "start from an example" affordance. Unlike
`feed.ExampleConfig()` (which `feed.Store.ConfigInfo` falls back to when
`profiles.yaml` doesn't exist yet), **nothing serves this automatically**:
its `source`/`feed`/`action` ids (`team-prs`/`team-review`/`review-pr`) are
placeholders that won't resolve against a real install's `profiles.yaml`/
`actions.yml`, so writing it to disk (or wiring it as the flows editor's
"New flow" template) would hand a user a permanently-broken flow rather than
a helpful one — see `internal/desktop/pipeline/flow/example_test.go` for the
test asserting it parses and validates clean against a `flow.MapRefs` that
resolves those same placeholder ids. The flows editor's actual "New flow"
action (`usePipelineEditor.ts`'s `newFlow`) starts genuinely blank
(`{nodes: [], wires: []}`) rather than from this template: seeding a
multi-node draft would mean instantiating five node types, wiring them, and
placing a layout position for each (well beyond a small, self-contained
change to `newFlow`), and — since a brand-new flow can't know which real
`source`/`feed`/`action` ids exist in a given install — it would start the
user off with unresolved-reference validation errors on every terminal node
rather than a genuinely blank canvas.

## Key files

| Concern | Path |
| --- | --- |
| Msg / event log / commit protocol | `internal/desktop/pipeline/pipelinedb/log.go`, `commit.go`, `db.go` |
| Migrations | `internal/desktop/pipeline/pipelinedb/migrations/`, `internal/data/migrate` |
| Source producer | `internal/desktop/pipeline/producer.go`, `source.go`, `github_source.go` |
| Output worker + executors | `internal/desktop/pipeline/output_worker.go`, `launch_session_executor.go`, `shell_executor.go`, `publish_event_executor.go` |
| `flows/*.yaml` schema | `internal/desktop/pipeline/flow/` |
| `actions.yml` schema | `internal/desktop/pipeline/actions/` |
| Wails services | `desktop/pipelineservice.go`, `desktop/flowsservice.go`, `desktop/flowsrefs.go`, `desktop/main.go` |
| Frontend node-type contract | `desktop/frontend/src/pipeline/nodeType.ts`, `registry.ts`, `nodes/*/` |
| Frontend engine | `desktop/frontend/src/pipeline/engine/` (`graph.ts`, `runGraph.ts`, `transport.ts`, `webWorkerTransport.ts`) |
| Frontend driver + composables | `desktop/frontend/src/pipeline/driver.ts`, `composables/usePipelineEditor.ts`, `composables/usePipelineRuntime.ts` |
| Flows editor UI | `desktop/frontend/src/pipeline/components/FlowsView.vue` and siblings |
| e2e | `desktop/e2e/tests/flows-editor.spec.ts`, `desktop/e2e/playwright.config.ts` |
