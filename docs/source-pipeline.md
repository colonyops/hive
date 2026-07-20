# The desktop source pipeline

!!! note "Current architecture and remaining gaps"
    The pipeline described here **is** the desktop app's feed system: a
    profile is a flow, and the sidebar reads persisted `feed_item` rows this
    pipeline commits. See
    [Current architecture and remaining gaps](#current-architecture-and-remaining-gaps)
    for the current runtime shape and remaining gaps.

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
             │ db.AppendIfChanged(...) + db.AppendSnapshot(...)
             ▼
 ┌────────────────────────────────────────────────────────────┐
 │ event_log  (desktop-pipeline.db — append-only, STRICT)      │
 └───────────┬──────────────────────────────────────────────┬─┘
             │ PipelineService.ReadFrom(consumer, limit)     │
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
   (flows editor's read-only preview          drains queued output   │
   panel AND the sidebar — both read          commands and dispatches │
   the same persisted rows)                   their typed executors   │
                                                                     │
   consumer_offset  ◀────────── ConsumerOffset / CommitBatch ────────┘
```

Delivery (`ReadFrom`/`FeedItems`) and batched commits are exposed to the
frontend by `desktop/pipelineservice.go`; the append-only write side
(`internal/desktop/pipeline`'s `Producer`/`Source`) never talks to the
frontend directly.

## The `msg` contract

Every value flowing through the log and the graph runtime is a `Msg`,
defined once in `internal/desktop/pipeline/pipelinedb/log.go` and re-exported
verbatim as `pipeline.Msg` (a type alias, `internal/desktop/pipeline/source.go`):

```go
type Msg struct {
	ID       string
	Key      string
	Topic    string
	Ts       int64
	Payload  json.RawMessage
	Snapshot []SnapshotItem `json:"Snapshot,omitempty"`
}
```

| Field     | Meaning                                                                 | Maps onto `event_log` as |
| --------- | ------------------------------------------------------------------------ | ------------------------ |
| `ID`      | Unique per log record — the engine's own commit cursor/dedup key         | `"offset"` (as a string) |
| `Key`     | The item's stable identity (e.g. `"colonyops/hive#2841"`), used for source identity and `feed_item`/`output_command` dedup | `key` |
| `Topic`   | `"source:<source-id>"` — which configured source produced this message   | `topic` |
| `Ts`      | Unix nanoseconds when the row was appended                               | `created_at` |
| `Payload` | The opaque item JSON (shape is set by the source, e.g. a PR/issue/notification); snapshot rows carry the encoded `SnapshotItem[]` here too | `payload` |
| `Snapshot` | Present only for authoritative successful source snapshot rows; contains the full current key/payload set for that source | decoded from snapshot rows (`snapshot != 0`) |

One casing/location fact worth calling out explicitly, since it trips
people up:

- **`Msg` serializes its core fields under literal Go field names**, unlike
  every other wire type in this codebase (`CommitBatch`, `Output`, `Sink`, …,
  all of which use lowerCamel JSON tags). `Snapshot` is explicitly tagged as
  capitalized to match. A function node's `on_message` body therefore reads
  `msg.Payload`, `msg.Key`, `msg.ID`, `msg.Topic`, `msg.Snapshot` —
  **capitalized**, not `msg.payload`/`msg.key`. See
  `desktop/frontend/src/pipeline/nodes/function/help.md`.

## The dedicated DB and table roles

The pipeline owns a separate SQLite database,
`internal/desktop/pipeline/pipelinedb` (`desktop-pipeline.db`, opened via
`pipelinedb.Open(desktop.StateDir(), …)`), deliberately isolated from hive's
shared `hive.db`. The package doc on `pipelinedb/db.go` states the reason
directly: desktop pipeline write traffic (a poll tick appending dozens of
rows, a commit batch running every pump) must never contend with the
CLI/TUI's own SQLite writer.

The base tables are declared `STRICT` (SQLite's opt-in type enforcement) in
`pipelinedb/migrations/0001_pipeline.up.sql`. Later migrations add the
action-dedup key, bounded retries, confirmation state, durable source heads,
and source snapshot reconciliation metadata:

| Table | Role |
| --- | --- |
| `event_log` | Append-only log of every message a source has ever produced. `"offset"` (quoted — a SQLite keyword) is the autoincrement primary key and the thing everything else replays from. Indexed on `(topic, "offset")`. Snapshot rows carry a successful source poll's full current item set. |
| `consumer_offset` | One row per consumer (a flow id), tracking the last offset that consumer's commit has fully accounted for. |
| `source_head` | Durable `(topic, key) → payload` source head used by `AppendIfChanged` to skip unchanged source item events across producer restarts. |
| `feed_item` | Go-owned, persisted output of a flow's `feed` nodes — primary key `(feed_id, item_id)`, so a re-commit of the same item is an upsert, not a duplicate row. Snapshot metadata (`source_topic`, `snapshot_id`) lets commits reconcile rows that disappear from a source. |
| `output_command` | The queue of side-effecting action invocations waiting for the output worker, deduped on `(action_id, key)` (unique index `idx_output_command_action_key`), with `status`/`attempts`/`last_error` for confirmation and bounded retry. Terminal-history pruning uses the partial `idx_output_command_terminal_id` index. |
| `node_run` | Per-node, per-tick execution metrics (`in_count`/`out_count`/`drop_count`/`ok`/`err`/`dur_ms`) for the flows editor's debug panel and RECENT list. `idx_node_run_flow_ended_at` serves the per-flow view and `idx_node_run_ended_at` serves retention pruning. |

These tables share the same migration runner, `internal/data/migrate` — a small,
storage-agnostic package (`Load`/`Apply`/`Up` over an `fs.FS` of
`NNNN_name.up.sql` files, tracked in a `schema_migrations` table) also used by
hive's own `hive.db`. `pipelinedb.Open` calls `migrate.Up` directly with no
legacy-bootstrap step, since this database has no pre-migration history to
reconcile.

## The log API

`pipelinedb.DB` (`log.go`) exposes the whole event-log surface:

- **`Append(ctx, topic, key, payload) (offset int64, err error)`** — inserts
  one ordinary event row, stamping `created_at` as `time.Now().UnixNano()`.
- **`AppendIfChanged(ctx, topic, key, payload) (offset int64, appended bool, err error)`** —
  atomically compares the payload against `source_head` for `(topic, key)`,
  appends only changed non-empty keys, and updates the head in the same
  transaction. Empty-key messages always append because they have no stable
  source-item identity.
- **`AppendSnapshot(ctx, topic, items) (offset int64, err error)`** — appends
  one authoritative full-source snapshot row for a successful poll, including
  an empty source.
- **`ReadFrom(ctx, offset, limit) ([]Msg, nextOffset int64, err error)`** —
  rows with `"offset" > offset`, ascending, up to `limit`. If nothing
  matches, `nextOffset` is the input `offset` unchanged, so a caller can
  always resume with `ReadFrom(ctx, nextOffset, limit)`.
- **`ReadForConsumer(ctx, consumer, limit) ([]Msg, error)`** — the Wails-facing
  read path: it resolves the consumer's SQLite checkpoint and returns the
  next page after it, so the frontend never owns a separate numeric cursor.
- **`ConsumerOffset(ctx, consumer) (int64, error)`** — reads a consumer's
  checkpoint. Checkpoints are advanced only through `CommitBatch`, whose
  monotonic SQL upsert means an out-of-order or replayed batch never regresses
  a consumer's checkpoint.

### Retention

`pipeline.Maintenance` runs every five minutes and resolves the enabled flow
IDs from `flow.FlowStore` on every pass. It calls `DB.Prune` transactionally:

- Event rows are removed only through the **minimum** durable offset of all
  enabled flows. No enabled flows, or one enabled flow without a
  `consumer_offset` yet, means no event-log rows are removed. Disabled flows
  intentionally do not hold the boundary; when re-enabled, they start with
  the then-current log.
- The newest 10,000 `node_run` records are retained globally.
- The newest 2,000 terminal (`done` or permanently `failed`) `output_command`
  records are retained. Nonterminal and retryable commands are never candidates
  for deletion.

The loop is stopped and joined as the desktop backend shuts down.

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
	Sink           Sink
	Key            string          // feed_item.item_id, and the output_command dedup key
	Payload        json.RawMessage
	Unread         bool            // feed items only
	SourceTopic    string          // feed snapshot reconciliation scope
	SnapshotID     string          // source snapshot offset, as a string
	PreserveUnread bool            // snapshot refreshes keep an existing read state
}

type FeedSnapshot struct {
	FeedID      string
	SourceTopic string
	SnapshotID  string
}

type Discard struct {
	MsgID  string
	NodeID string
}

type CommitBatch struct {
	Consumer      string         // event_log consumer key — the flow id
	UpToOffset    string         // decimal event-log offset; preserves int64 across Wails
	Outputs       []Output
	FeedSnapshots []FeedSnapshot
	Discards      []Discard
	NodeRuns      []NodeRunView
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
   action `Output` into `output_command`, reconcile any `FeedSnapshot` scopes
   by deleting rows not produced by the snapshot, insert every `NodeRunView`
   into `node_run`, and advance `consumer_offset` to `UpToOffset` — all in
   the same transaction as an `INSERT … ON CONFLICT … WHERE excluded."offset" >
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
  an unresolved action reference, an out-of-range `outputs`/`timeout`); a wire
  naming an unknown node; a wire sourced from a
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
runtime implementation; tests use local fakes). `source`/`feed` used to
resolve against legacy profiles files too, before the cutover — both are
self-contained now (a `github-source` node embeds its own fetch config, a
`feed` node's identity is just its own node id), so `Refs` shrank down to the
one method.

Each flow file has a machine-written sibling layout file, `<id>.ui.yaml`
(`flow/layout.go`): node canvas positions only, keyed by node id. It is
purely cosmetic — never consulted by `LoadFlow`/validation, and a missing or
broken layout file is not an error (the editor just lays nodes out fresh).
`LoadFlows` explicitly skips `*.ui.yaml`/`*.ui.yml` files when scanning the
flows directory for flow definitions.

A second machine-written sibling, `<id>.sidebar.yaml` (`flow/sidebar.go`),
records how the profile's feed nodes are grouped into **folders** and ordered
in the sidebar's FEEDS section (`SaveSidebar`/`GetSidebar` on `FlowsService`,
driven by drag-and-drop in `SideBar.vue`). It stores **structure and order
only** — folder names and their member/top-level feed ids — not view state:
whether a folder is currently expanded is transient UI, kept in `localStorage`
via VueUse `useStorage` (keyed by flow id → folder ids), never in the file. So
`SidebarFolder` has no `collapsed` field, and collapsing a folder never writes
to disk. Like the layout file it is node-id-keyed, purely cosmetic, and skipped
by `LoadFlows`; a missing or broken file falls back to listing feeds in
flow-node order. It is a *separate* file from `.ui.yaml` so the canvas editor's
whole-layout writes can never clobber the sidebar grouping, and — unlike
`.ui.yaml` — the `FlowsWatcher` deliberately **ignores** `.sidebar.yaml`
(`isFlowFile`): a sidebar-layout write is frontend-owned UI state, so reloading
the store and emitting `flows:updated` for it would pointlessly blank and
refetch the sidebar (a visible flash on every reorder). The frontend resolves
the file against the flow's live feed nodes in `lib/feedTree.ts` — appending
feeds the file doesn't mention (so a newly added feed is never hidden) and
dropping references to feeds that no longer exist.

**Worked example** (a similar package-local fixture — with
`msg.payload` written lowercase, since it's a pure YAML round-trip test that
never actually executes the JS — lives in `flow/loader_test.go`'s
`workedExampleYAML`):

```yaml
version: 1
name: Frontend Triage
nodes:
  - { id: in-prs, type: github-source, kind: search, query: "is:open is:pr archived:false" }
  - { id: drop-bots, type: github-filter, exclude_authors: ["*[bot]"], repos: ["colonyops/*"] }
  - id: tag
    type: function
    outputs: 2
    on_message: |
      if (msg.Payload.state === "closed") return null;
      msg.Payload.tag = "review"; return [msg, null];
  - { id: team-feed, type: feed }
  - { id: spawn-review, type: action, action: review-pr }
wires:
  - { from: in-prs, to: drop-bots }
  - { from: drop-bots, to: tag }
  - { from: tag, out: 0, to: team-feed }
  - { from: tag, out: 0, to: spawn-review }
```

### `actions.yml`

Implemented by `internal/desktop/pipeline/actions`, at
`desktop.ActionsPath()` — **`$XDG_CONFIG_HOME/hive/desktop/actions.yml`**,
in the desktop config root, *not* a repo-scoped `.hive/actions.yml`. The
design doc calls this file `.hive/actions.yml`, but the desktop app's config
is global rather than tied to any one repo, so `ActionsPath()`'s doc comment
is explicit that it deliberately lives beside the desktop's flows config
instead. (`EnvActionsPath` — `HIVE_DESKTOP_ACTIONS` — overrides the location
outright.)

The package mirrors `flow`'s own registry + two-pass strict decode:

```yaml
version: 1
actions:
  - id: review-pr
    label: Spawn review agent
    type: launch-session       # | shell | publish-message
    applies_to: [pr]           # optional detail-pane kind scope
    show_in_detail: true       # presentation only; flow nodes still target it
    prompt_template: "Review {{ .Payload.title }}"
    repo_template: "git@github.com:colonyops/hive.git"
```

| `type` | Config fields | Executor |
| --- | --- | --- |
| `launch-session` | `prompt_template` (required, Go template), `agent?`, `repo_template?` | `LaunchSessionExecutor` creates a Hive session. A repository template is headless-capable; without one the detail pane asks for repository, name, and agent input. |
| `shell` | `command_template` (required), `cwd?`, `timeout?`, `env?` | `ShellExecutor` runs `sh -c <rendered command>` for trusted action authors. Stdout and stderr diagnostics are retained with a 64 KiB bound. |
| `publish-message` | literal `topic` and `message_template` (both required) | `PublishMessageExecutor` durably publishes the rendered payload with sender `hive-desktop` and no session id. Topics cannot be templates or wildcards. |

`id`/`label` are required envelope fields; `id` follows the same slug rule
as flow node ids. The global catalog lives in `actions.yml` and supports
create, edit, delete, and external-file reload. Invalid external YAML leaves
the prior last-good catalog active until a fixed file reloads. `show_in_detail`
controls only manual detail-pane visibility; it does not limit an `action`
flow node. Detail actions and flow outputs both use durable `(action_id, key)`
deduplication. Their typed result is either a launched session or a published
message; failed runs persist bounded diagnostics for later readback.

`actions.ActionStore` (`actions/store.go`) has the same last-good-on-failure
posture as `flow.FlowStore`: a broken `actions.yml` on reload keeps serving
whatever last parsed cleanly, rather than blanking every action out from under
a running flow. `actions.ActionsWatcher` (`actions/watcher.go`) watches the
`actions.yml` parent directory so hand edits apply live.

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

- `WebWorkerTransport` (`engine/webWorkerTransport.ts`) is the production
  default: `PipelineDriver` constructs it through `workerFactory.ts` when no
  test transport is injected. One **shared** worker hosts every
  `isolate: false` runtime (`github-filter` today); a **dedicated** worker is
  spawned per `isolate: true` node *instance* (`function`), so a timeout's
  `terminate()` kills only that one instance, never a sibling or the shared
  worker. It owns the message-protocol glue (request/response correlation,
  timeout-driven worker replacement, and state merged back across the
  `postMessage` structured-clone boundary).
- `InProcessTransport` runs a `ProcessorRuntime` directly on the calling
  thread, wrapped in a `Promise`, and enforces `timeoutMs` itself by racing
  the promise against a deadline. It remains an explicit unit-test/fallback
  implementation; callers must inject it instead of relying on it as the
  production default.
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

"Drain in-flight, then swap" is enforced by `useFlowsSession.ts`'s serialized
operation tail. Editor changes mutate only the selected draft; deployed
runtimes own independent cloned snapshots for every enabled flow. A Deploy
saves the draft through `FlowsService.SaveFlow`, then reloads that flow's
runtime snapshot (or stops it if disabled) through the same serialized queue
used by log drains and `flows:updated` reconciliation, so a replacement graph
is not installed halfway through an older commit.

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

**`pipeline.Worker`** (`output_worker.go`) is the output side: it drains up to
`DefaultOutputWorkerBatch` (50) runnable `output_command` rows by ID. A
matching detail-pane action explicitly creates and executes one durable
command. A failed background execution is retried (with `last_error`
recorded) until `MaxOutputCommandAttempts` (5), then marked permanently
`failed`; an unknown `action_id` (for example after a catalog edit) fails
immediately. Explicit detail actions are one-shot: attempted failures retain
bounded stdout/stderr diagnostics and are terminal rather than replayed
without the user's interactive input.

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
- **Playwright e2e** (`desktop/e2e`) — `mise run desktop:e2e` is Docker-only:
  the digest-pinned Playwright image builds a real server and runs feed,
  onboarding, pipeline, and action-smoke projects on private ports. A fresh
  256-bit Docker harness marker is required before either Playwright or the
  server launcher runs. Every server has a fresh data/config root.
  Fixture-driven servers receive private flows/actions copies and a run id;
  onboarding deliberately has no injected fixture env, and action-seed
  deliberately has no action fixture so it can prove first-run seeding. Action
  smoke also gets a local bare remote. Checked-in fixtures are never writable,
  and no project can share SQLite/action state. The action-smoke endpoint is
  read-only, requires action-smoke mode plus the Docker marker/run id, and only
  exposes its filtered command records; browser tests invoke Wails methods and
  UI before using it for durable readback.
  `feed.spec.ts`, `theme.spec.ts`, `onboarding.spec.ts`,
  `flows-editor.spec.ts`, `source-to-commit.spec.ts`, and `actions.spec.ts`
  cover the real persisted sidebar, graph commit, catalog UI, and action
  feedback without pixel assertions.

## Current architecture and remaining gaps

The source pipeline is no longer a parallel system running alongside the
legacy feed — it **is** the feed. A profile is a flow, the sidebar reads
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
  `profiles.yaml` that no longer exists. See [Starter flow](#starter-flow).
- **The flows editor is a per-profile sub-view, not a separate app mode.**
  `App.vue` no longer has a `mode` ref toggling `'feed'`/`'flows'`; it
  renders either `SideBar`+`FeedList` or `FlowsView` based on
  `useFlowsSession().flowsOpen` (a shared session's `shallowRef<boolean>` —
  see below), reached from the sidebar's "Flows" pill/footer row or the
  ⌘K jump-to-node command (there is no per-feed "reveal in flow" / edit
  affordance in the sidebar; a feed is edited by opening its node in the
  canvas), and exited via the titlebar breadcrumb (`TitleBar.vue`'s
  `data-testid="breadcrumb-profile-name"` becomes a button back to the feed
  while flows are active).
- **The sidebar reads `feed_item`.** `useFeedState.ts`'s `loadFeeds`/
  `loadItems` call `PipelineService.FeedItemCounts`/`FeedItems` — the same
  read path the flows editor's preview panel always used — instead of
  the deleted legacy feed store.
- **An always-on runtime manager keeps every enabled flow's `feed_item`
  current with the canvas closed.**
  `desktop/frontend/src/pipeline/composables/useFlowsSession.ts` is a
  module singleton, first constructed by `App.vue`, that owns editor state
  (`usePipelineEditor`) plus one `usePipelineRuntime` per enabled, valid
  flow. Each runtime consumes its own durable flow-id offset. Profile and
  canvas selection choose only the editor draft; they never start, stop, or
  redirect deployed processing. `flows:updated` refreshes the listing,
  starts new enabled flows, stops disabled/deleted flows, and atomically
  replaces every surviving deployed snapshot after earlier drains finish.
  `App.vue` subscribes once to `"log:appended"`, calls `session.pump()` for
  all runtimes, then calls `useFeedState`'s `refresh()`, preserving
  commit-then-refresh ordering for every profile sidebar. `FlowsView.vue`
  reads the same session and never creates a competing runtime.
- **`github-source` embeds its GitHub fetch config directly.**
  `GithubSourceConfig` (`internal/desktop/pipeline/flow/nodes_source.go`) is
  `{kind, query?, limit?}` — a `github-source` node is self-contained
  rather than naming an external source id. A `feed` node is likewise
  config-free: `FeedConfig` is an empty struct, and the feed's identity is
  just its own (flow-qualified) node id.
- **`rpc-source` is gone**, not merely unimplemented: it has been removed
  from `flow`'s node registry (`flow/node.go`) along with its schema type.
  There is no RPC source today, backend or frontend.
- **The legacy feed system's `FilterDef`/`fetchSourceDirect` orchestration,
  its `Store` (readstate), its poller, and its mock provider are all
  deleted.** `internal/desktop/feed` now holds only the `Item` wire type and
  `LiveProvider.SourceItems` — the cached, conditional, singleflight GitHub
  fetch core the pipeline's `githubSource` producer calls through. Configured
  actions are exposed separately as `actions.View` from `ActionStore`.
  Nothing about client-side filtering survived the cutover; a
  `github-filter` flow node is the only filtering mechanism now.

**What's still incomplete**, independent of the cutover above:

- The runtime records completed `node_run` rows but does not expose a real
  per-node in-flight signal, so the canvas and debug strip cannot show which
  individual node is currently executing.
- Docker e2e exercises the real `feed_item`/flow-backed sidebar, frontend
  graph commit, and action catalog through private deterministic fixtures;
  screenshot assertions remain intentionally out of scope because behavior
  and persisted field values are the contract.

## Starter flow

**`flow.FlowStore.starterFlow`** (private to
`internal/desktop/pipeline/flow/store.go`) is what a user gets when they
create a profile: `FlowStore.Create` — the backend of "New profile" — seeds
it automatically, and every field is real and immediately usable (three
`github-source` nodes with real, pollable `kind`/`query` configs, each wired
straight to its own `feed` node: My open PRs / Assigned / Notifications).
This is the default flow that runs automatically for a new profile. It is a
safe per-profile starter instead of a global template that tries to
demonstrate every node type.

A separate affordance: the flows editor's own "New flow name…" field (in
the canvas toolbar's flow-selector dropdown) calls `usePipelineEditor.ts`'s
`newFlow`, which starts a genuinely blank, unsaved draft (`{nodes: [],
wires: []}`) rather than a template. Nothing is written until the user adds
nodes by hand and clicks Deploy. This is a lower-level "add another flow"
escape hatch alongside the starter-seeded "New profile" path (a deployed
blank-started flow is still just a flow file, so it still shows up as its
own profile tile).

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
| Sidebar (profile = flow, `feed_item` reads) | `desktop/frontend/src/composables/useFeedState.ts`, `desktop/frontend/src/components/SideBar.vue`, `SidebarFeedRow.vue` |
| Sidebar feed folders + ordering | `internal/desktop/pipeline/flow/sidebar.go` (`<id>.sidebar.yaml`), `desktop/frontend/src/lib/feedTree.ts` |
| Always-on per-enabled-flow runtime manager | `desktop/frontend/src/pipeline/composables/useFlowsSession.ts`, `usePipelineRuntime.ts` |
| Frontend node-type contract | `desktop/frontend/src/pipeline/nodeType.ts`, `registry.ts`, `nodes/*/` |
| Frontend engine | `desktop/frontend/src/pipeline/engine/` (`graph.ts`, `runGraph.ts`, `transport.ts`, `webWorkerTransport.ts`) |
| Frontend driver + composables | `desktop/frontend/src/pipeline/driver.ts`, `composables/usePipelineEditor.ts`, `composables/usePipelineRuntime.ts` |
| Flows editor UI | `desktop/frontend/src/pipeline/components/FlowsView.vue` and siblings |
| e2e mock fixtures | `desktop/mockseed.go`, `desktop/e2e/fixtures/flows/` |
| e2e | `desktop/e2e/tests/`, `desktop/e2e/scripts/serve.sh`, `desktop/e2e/playwright.config.ts` |
