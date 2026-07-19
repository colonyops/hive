# The desktop source pipeline

!!! note "Current architecture — a handful of pieces still incomplete"
    The pipeline described here **is** the desktop app's feed system: a
    profile is a flow, and the sidebar reads persisted `feed_item` rows this
    pipeline commits. See
    [Current architecture and remaining gaps](#current-architecture-and-remaining-gaps)
    for what's still not wired up.

The source pipeline is a Node-RED-style graph editor and runtime built into
Hive Desktop: GitHub sources feed an append-only event log, a stateless
frontend graph runtime processes it in small batches, and the result is
committed back to Go as persisted feed items and side-effecting actions. A
profile IS a flow — the rail's profile tiles, the sidebar's feeds, and the
items in the feed list are all backed by a `flows/*.yaml` file and the
`feed_item` rows its `feed` nodes commit. The graph editor itself (the
Node-RED-style canvas) is reachable via the command palette (`⌘K` →
**"Edit flow…"**), the sidebar's "Flows" pill, or a feed row's "Reveal in
flow" icon — a per-profile sub-view, not a separate app mode.

This document is the durable architecture reference for that system. It
covers the wire contract, the storage layer, the config schemas, the
frontend node-type contract, and — now that the cutover to this being the
sidebar's actual data source has landed — what still needs to happen beyond
that (a handful of gaps, not a parallel system left to retire).

## Overview and the tripartite model

The pipeline is split into three tiers that never share a database or a
process boundary:

1. **Go, write side (source producer)** — polls GitHub and appends whatever
   it finds to an append-only log, deduplicating by key so an unchanged item
   isn't re-appended every tick. (An `rpc-source` node type once existed as a
   schema-only placeholder; it has since been removed entirely — see
   [Current architecture and remaining gaps](#current-architecture-and-remaining-gaps).)
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
       │  internal/desktop/feed's surviving GitHub fetch core)
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
   (flows editor's read-only preview          drains output_command, │
   panel AND the sidebar — both read          auto_apply gate,       │
   the same persisted rows)                   dispatches Executors    │
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

## The config files and schemas

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
| `github-source` | 0 / 1 | `kind` (`search`\|`notifications`), `query?` (required for `search`), `limit?` | Backend-run — see [source producer](#source-producer-and-output-worker). Embeds its own GitHub fetch config directly; no cross-file reference. |
| `github-filter` | 1 / 2 | `repos`/`exclude_repos`/`authors`/`exclude_authors`/`labels`/`exclude_labels` (doublestar globs), `types` (`pr`\|`issue`), `reasons` (GitHub notification reasons) | Port 0 = pass, port 1 = fail. At least one filter group must be set (a hard error if all are empty). |
| `function` | 1 / 1..16 (`outputs`) | `on_message` (required), `on_start?`, `on_stop?`, `outputs?` (1–16, default 1), `timeout?` (100ms–60s, default 5s) | Author-trusted JS, no sandbox. |
| `feed` | 1 / 0 (terminal) | *(none)* | Commits `Sink{feed, "<flowId>/<nodeId>"}`; new items land unread. The node's own (flow-qualified) id IS the feed's identity — there's no field to set. |
| `action` | 1 / 0 (terminal) | `action` (an id in `actions.yml`) | Commits `Sink{action, <action id>}`. |

`rpc-source` no longer exists: it has been removed from `flow`'s node
registry (`flow/node.go`) along with its schema type, not merely left
unimplemented.

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

The only remaining cross-file reference is the `action` node's `action` id,
resolved through an injected `flow.Refs` interface (`ResolveAction(id)
bool`) — the package never imports the actions loader itself, so it can be
wired in independently (`desktop/flowsrefs.go`'s `actionsRefs` is the real
implementation; `flow.MapRefs` is the map-backed test double used throughout
`flow`'s own test suite). `source`/`feed` used to resolve against
`profiles/*.yml` too, before the cutover — both are self-contained now (a
`github-source` node embeds its own fetch config, a `feed` node's identity
is just its own node id), so `Refs` shrank down to the one method.

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

### Legacy `profiles.yaml` and the `migrate` converter

`profiles.yaml` (`sources:` + `profiles:` → `feeds:`, each feed's own
`filters:` block) is **not a live schema any more** — nothing in the running
app reads it, and the old `internal/desktop/feed` code that used to parse it
(`FilterDef`, `Store`, `ConfigInfo`, the poller, …) is deleted. It survives
purely as the *input* format `internal/desktop/migrate` converts, one time,
into `flows/*.yaml`.

`internal/desktop/migrate` (run via the desktop binary's
`--migrate-profiles[=dry|write]` flag, wired up in `desktop/main.go`'s
`runMigrationIfRequested`) keeps its own **private** copy of the old config
shape (`legacyConfig`/`legacySource`/`legacyFilter`/`legacyFeed`/
`legacyProfile` in `migrate/convert.go`), decoded laxly so a field the app no
longer knows about is ignored rather than fatal — that isolation is what let
the legacy feed package's config schema be deleted outright without
`migrate` needing any of it.

Per legacy profile, `buildFlow` produces one flow: each legacy `feed`
becomes a `feed` node (plus a `github-filter` node when the feed had a
non-empty `filters:` block), and each `source` a feed referenced becomes —
once, deduplicated — a `github-source` node carrying that source's
`kind`/`query`/`limit` as its own embedded `GithubSourceConfig`, all laid out
in three columns (sources · filters · feeds). The produced flow is rendered,
validated against a real `flow.MapRefs{}`, and — in write mode — saved via
`flow.SaveFlow`/`flow.SaveUI` next to a one-time `.bak` backup of the
original `profiles.yaml`; `--force` is required to overwrite a
`flows/<id>.yaml` that already exists, so a re-run never clobbers hand
edits. See
[Current architecture and remaining gaps](#current-architecture-and-remaining-gaps)
below for how this fits into the cutover as a whole.

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
`desktop/frontend/src/pipeline/nodes/<type>/`, one directory per type:
`action`, `feed`, `function`, `github-filter`, `github-source`. (An earlier
revision of this document also listed a schema-only `rpc-source` type; it
has since been removed entirely — see
[Current architecture and remaining gaps](#current-architecture-and-remaining-gaps).)

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

- `'source'` — backend-run (`github-source` is the only type today). No
  `runtime.ts` at all; the frontend only relays whatever the backend
  producer already appended, filtering entry-node messages by the node's
  own flow-qualified log topic (`"source:<flowId>/<nodeId>"`).
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
   `github-source` node only accepts messages on its own flow-qualified
   `"source:<flowId>/<nodeId>"` topic; a bare entry node with no matching
   source type accepts everything (useful for tests exercising one node in
   isolation). Anything matching zero entry nodes becomes an immediate
   discard (`UNROUTED_NODE_ID`).
3. Walk nodes in topological order, per pending message: `disabled` nodes
   discard; a `github-source` node passes straight through to its wires;
   terminal (`feed`/`action`) nodes become an `Output`; everything else runs
   through the transport.
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
tick, so a `github-source` node added to, edited in, or removed from any
flow takes effect without a restart), calls `Produce` on each `Source`, and
appends every emitted `Msg`. `NewFlowSourceLister`
(`internal/desktop/pipeline/github_source.go`) builds one `githubSource` per
enabled `github-source` node across every loaded flow
(`pipeline.FlowLister`, satisfied by `*flow.FlowStore`), keyed and
topic-tagged by that node's flow-qualified id (`"<flowId>/<nodeId>"`, topic
`"source:<flowId>/<nodeId>"`) — there is no more `profiles.yaml` sources
list to enumerate. `githubSource` itself doesn't fetch GitHub — it delegates
to `feed.LiveProvider.SourceItems` (`internal/desktop/feed/live.go`), the
cached/conditional/singleflight fetch path that's now all that's left of the
once-larger `internal/desktop/feed` package, built from a `feed.SourceDef`
constructed on the fly from the node's own embedded `GithubSourceConfig`
rather than loaded from a config file. Two `github-source` nodes with
identical fetch config still share one GitHub request (`LiveProvider` keys
its cache on kind+query+limit, not id) while producing distinct topics so
each flow's graph only ingests its own rows. A source fetch failure is
logged and skipped; it never blocks other sources in the same tick.

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
  non-default port). The `feed`-mode server additionally points
  `HIVE_DESKTOP_FLOWS` at a checked-in fixture directory
  (`desktop/e2e/fixtures/flows/`, one flow) and seeds a matching, fixed set
  of `feed_item` rows at startup (`desktop/mockseed.go`, gated on
  `desktop.MockMode() == "feed"`) — this replaced
  `internal/desktop/feed/mock.go`'s static in-memory item list once the
  sidebar switched onto real `feed_item` reads (see
  [Current architecture and remaining gaps](#current-architecture-and-remaining-gaps)).
  `desktop/e2e/tests/flows-editor.spec.ts` exercises the flows editor
  surface against that same fixture flow (opening it from the command
  palette, the populated canvas, the node palette); `feed.spec.ts`,
  `theme.spec.ts`, and `onboarding.spec.ts` cover the sidebar/detail pane,
  theming, and first-run onboarding plus profile/flow-node CRUD
  respectively. Deeper coverage — a stateless-commit/replay spec, and
  screenshot snapshots for the flows canvas — is still deferred future
  work; see the next section.

## Current architecture and remaining gaps

The cutover earlier revisions of this document described as "deferred" has
landed: the source pipeline is no longer a parallel system running alongside
the legacy feed — it **is** the feed. A profile is a flow, the sidebar reads
`feed_item`, and the old `profiles.yaml`-based feed/source/filter machinery
is gone.

- **A profile IS a flow.** The rail's profile tiles are flow files
  (`FlowsService.ListFlows`/`GetFlow`), and "New profile"
  (`NewProfileModal` → `useFeedState.createProfile` →
  `FlowsService.CreateFlow`) calls `flow.FlowStore.Create`, which seeds a
  real, resolvable **starter flow** (`FlowStore.starterFlow`,
  `internal/desktop/pipeline/flow/store.go`) — three `github-source` →
  `feed` pairs (My open PRs / Assigned / Notifications), each source
  embedding its own fetch config directly rather than pointing at a
  `profiles.yaml` that no longer exists. This directly addresses what
  earlier revisions of this document listed as deferred work ("a default
  flow that runs automatically") — it landed in this per-profile shape
  rather than as `flow.ExampleFlow()`'s worked example — see
  [Starter-flow example](#starter-flow-example) for how the two differ.
- **The flows editor is a per-profile sub-view, not a separate app mode.**
  `App.vue` no longer has a `mode` ref toggling `'feed'`/`'flows'`; it
  renders either `SideBar`+`FeedList` or `FlowsView` based on
  `useFlowsSession().flowsOpen` (a shared session's `shallowRef<boolean>` —
  see below), reached from the sidebar's "Flows" pill/footer row, a
  jump-to-node command, or a feed row's "Reveal in flow" icon
  (`SideBar.vue`'s `data-testid="sidebar-reveal-in-flow"` — there is no
  pencil/edit affordance any more; a feed is edited by opening its node in
  the canvas), and exited via the titlebar breadcrumb (`TitleBar.vue`'s
  `data-testid="breadcrumb-profile-name"` becomes a button back to the feed
  while flows are active).
- **The sidebar reads `feed_item`.** `useFeedState.ts`'s `loadFeeds`/
  `loadItems` call `PipelineService.FeedItemCounts`/`FeedItems` — the same
  read path the flows editor's preview panel always used — instead of
  `internal/desktop/feed.Store`, which no longer exists.
- **An always-on, per-profile runtime keeps `feed_item` current with the
  canvas closed.**
  `desktop/frontend/src/pipeline/composables/useFlowsSession.ts` is a
  module singleton, first constructed by `App.vue`, that owns both the
  editor state (`usePipelineEditor`) and a `usePipelineRuntime` instance for
  whichever flow is bound to the active profile
  (`session.bindActiveFlow(activeProfileId)`, watched in `App.vue`).
  `App.vue` subscribes to the backend's `"log:appended"` event once, at the
  app level, and on every tick calls `session.pump()` and *then*
  `useFeedState`'s `refresh()` — commit-then-refresh ordering is the whole
  point, so the sidebar never reads a stale `feed_item` row.
  `FlowsView.vue` reads the same session, so opening the canvas never
  starts a second, competing runtime. This runs only the **active**
  profile's flow today; running every enabled profile's flow concurrently
  is a follow-up (search the frontend for `hc-8ft4yhm6`).
- **`github-source` embeds its GitHub fetch config directly.**
  `GithubSourceConfig` (`internal/desktop/pipeline/flow/nodes_source.go`) is
  `{kind, query?, limit?}` — a `github-source` node no longer names a
  `profiles.yaml` source id, because that file is no longer a live schema
  (see [Legacy `profiles.yaml` and the `migrate` converter](#legacy-profilesyaml-and-the-migrate-converter)
  above). A `feed` node is likewise config-free: `FeedConfig` is an empty
  struct, and the feed's identity is just its own (flow-qualified) node id.
- **`rpc-source` is gone**, not merely unimplemented: it has been removed
  from `flow`'s node registry (`flow/node.go`) along with its schema type.
  There is no RPC source today, backend or frontend.
- **The legacy feed system's `FilterDef`/`fetchSourceDirect` orchestration,
  its `Store` (readstate), its poller, and its mock provider are all
  deleted.** `internal/desktop/feed` now holds only what the pipeline still
  needs: the `Item`/`Action` wire types and `LiveProvider.SourceItems` — the
  cached, conditional, singleflight GitHub fetch core the pipeline's
  `githubSource` producer calls through. Nothing about client-side
  filtering survived the cutover; a `github-filter` flow node is the only
  filtering mechanism now.

**What's still incomplete**, independent of the cutover above:

- `WebWorkerTransport` is fully implemented and unit-tested but not wired
  into the running app — `InProcessTransport` (main-thread) is what
  actually executes every processor node today.
- `launch-session` and `publish-event` actions execute against **logging
  stubs** (`LoggingSessionLauncher`, `LoggingEventPublisher`) — only `shell`
  actually does anything outside a log line.
- "Drain in-flight, then swap" Deploy semantics are still realized more by
  construction than by an explicit drain step — see
  [Deploy and drain semantics](#deploy-and-drain-semantics) above; this
  didn't change with the cutover.
- There is still no manual confirmation UI for a non-`auto_apply` action
  sitting `pending` in `output_command`.
- e2e screenshot snapshots and a stateless-commit/replay spec are still
  deferred future work requiring the Docker-based `mise run desktop:e2e`
  gate to verify against real rendered output. What changed this phase is
  that the existing suites (`feed.spec.ts`, `theme.spec.ts`,
  `flows-editor.spec.ts`, `onboarding.spec.ts`) now exercise the real
  `feed_item`/flow-backed sidebar via a deterministic mock seed
  (`desktop/mockseed.go` + `desktop/e2e/fixtures/flows/`) instead of the
  deleted `internal/desktop/feed/mock.go`'s static fixture — see
  [Testing strategy](#testing-strategy) above.

## Starter-flow example

There are now two different "starter flow" concepts worth telling apart:

- **`flow.ExampleFlow()`** (`internal/desktop/pipeline/flow/example.go`) is
  a concrete, commented, worked `flows/*.yaml` document — the same
  github-source → github-filter → function(outputs: 2) → {feed, action}
  pipeline described above — kept purely for docs (this file) and tests.
  **Nothing serves this automatically**: its `action` id (`review-pr`) is a
  placeholder that won't resolve against a real install's `actions.yml`, so
  writing it to disk would hand a user a permanently-broken flow rather than
  a helpful one — see `internal/desktop/pipeline/flow/example_test.go` for
  the test asserting it parses and validates clean against a `flow.MapRefs`
  that resolves that same placeholder id. (Before the cutover its
  `source`/`feed` fields were placeholders too, resolved against
  `profiles.yaml`; now that `github-source`/`feed` are self-contained, only
  the `action` reference is left unresolved.)
- **`flow.FlowStore.starterFlow`** (private to
  `internal/desktop/pipeline/flow/store.go`) is what a user actually gets:
  `FlowStore.Create` — the backend of "New profile" — seeds it
  automatically, and every field is real and immediately usable (three
  `github-source` nodes with real, pollable `kind`/`query` configs, each
  wired straight to its own `feed` node: My open PRs / Assigned /
  Notifications). This is the "default flow that runs automatically" that
  earlier revisions of this document listed as deferred work — it landed in
  this shape (seeded per new profile, not a single global default) rather than as
  `flow.ExampleFlow()`'s worked example, since the worked example's whole
  point is demonstrating every node type (including
  `function`/`github-filter`/`action`), not being a safe, immediately
  runnable starting point.

A third, unrelated affordance: the flows editor's own "New flow name…"
field (in the canvas toolbar's flow-selector dropdown) calls
`usePipelineEditor.ts`'s `newFlow`, which starts a genuinely blank, unsaved
draft (`{nodes: [], wires: []}`) rather than either template above —
nothing is written until the user adds nodes by hand and clicks Deploy.
This is a lower-level "add another flow" escape hatch alongside the
starter-seeded "New profile" path (a deployed blank-started flow is still
just a flow file, so it still shows up as its own profile tile) — seeding
it from `flow.ExampleFlow()` would run into the same problem
`ExampleFlow()` always has: a brand-new flow can't know which real `action`
id (if any) exists in a given install, so a multi-node template risks
starting the user off with unresolved-reference errors rather than a
genuinely blank canvas.

## Key files

| Concern | Path |
| --- | --- |
| Msg / event log / commit protocol | `internal/desktop/pipeline/pipelinedb/log.go`, `commit.go`, `db.go` |
| Migrations | `internal/desktop/pipeline/pipelinedb/migrations/`, `internal/data/migrate` |
| Source producer | `internal/desktop/pipeline/producer.go`, `source.go`, `github_source.go` |
| Output worker + executors | `internal/desktop/pipeline/output_worker.go`, `launch_session_executor.go`, `shell_executor.go`, `publish_event_executor.go` |
| `flows/*.yaml` schema | `internal/desktop/pipeline/flow/` |
| `actions.yml` schema | `internal/desktop/pipeline/actions/` |
| `profiles.yaml` → `flows/*.yaml` migration | `internal/desktop/migrate/` |
| Wails services | `desktop/pipelineservice.go`, `desktop/flowsservice.go`, `desktop/flowsrefs.go`, `desktop/main.go` |
| Sidebar (profile = flow, `feed_item` reads) | `desktop/frontend/src/composables/useFeedState.ts`, `desktop/frontend/src/components/SideBar.vue` |
| Always-on per-profile runtime | `desktop/frontend/src/pipeline/composables/useFlowsSession.ts`, `usePipelineRuntime.ts` |
| Frontend node-type contract | `desktop/frontend/src/pipeline/nodeType.ts`, `registry.ts`, `nodes/*/` |
| Frontend engine | `desktop/frontend/src/pipeline/engine/` (`graph.ts`, `runGraph.ts`, `transport.ts`, `webWorkerTransport.ts`) |
| Frontend driver + composables | `desktop/frontend/src/pipeline/driver.ts`, `composables/usePipelineEditor.ts`, `composables/usePipelineRuntime.ts` |
| Flows editor UI | `desktop/frontend/src/pipeline/components/FlowsView.vue` and siblings |
| e2e mock fixtures | `desktop/mockseed.go`, `desktop/e2e/fixtures/flows/` |
| e2e | `desktop/e2e/tests/`, `desktop/e2e/scripts/serve.sh`, `desktop/e2e/playwright.config.ts` |
