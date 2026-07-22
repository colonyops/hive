# The desktop source pipeline

Hive Desktop is an inbox-first system for GitHub observations. Each profile is
a flow. Sources collect observations, Go classifies and durably persists them,
and the frontend flow engine decides which unarchived items belong to each
sidebar feed. This separation keeps an item’s identity and triage state stable
while flows, filters, and feed membership change.

## Architecture

The pipeline has three cooperating parts:

1. **Go ingestion** polls configured `github-source` nodes. For every changed
   observation it classifies the change and unconditionally updates the
   corresponding `inbox_item`; noteworthy classifications also append an
   `inbox_event`. Ingestion owns item identity, payload, revision, lifecycle,
   unread state, and archive state.
2. **The frontend engine** evaluates each enabled flow. Its ordinary event-log
   pass advances a durable consumer offset, records node-run diagnostics, and
   can enqueue deduplicated actions. On startup and deploy, its synthetic replay
   evaluates each source node's latest authoritative snapshot and resolves
   outputs against the current unarchived inbox. It then atomically installs
   those membership claims and fast-forwards the consumer to the captured log
   tail without enqueuing actions.
3. **The desktop UI** reads inbox views and feed membership claims. It provides
   triage controls over the same durable inbox state rather than maintaining a
   separate read-state store.

```
GitHub API
   │
   ▼
github-source → Go producer → classify and persist
                               │
                 ┌─────────────┴─────────────┐
                 ▼                           ▼
            inbox_item                   inbox_event
                 │                           │
                 ▼                           ▼
      frontend flow engine             observed history
                 │
                 ▼
     feed_membership_claim ───► inbox and feed views
                 │
                 └────────────► deduplicated action queue
```

An item is never owned by a feed. A flow may claim an item for one or more of
its feeds, and an item with no claim appears in the **Unfiled** view. This is
why deploying a changed filter can recompute visible membership without
rewriting the item or repeating an action.

## Storage and retention

The pipeline uses its own SQLite database, `desktop-pipeline.db`, opened by
`internal/desktop/pipeline/pipelinedb`. It is separate from `hive.db` so
pipeline polling and desktop interactions do not compete with CLI/TUI writes.
All timestamps stored by this database are Unix milliseconds.

| Table | Purpose | Retention / cascade behavior |
| --- | --- | --- |
| `inbox_item` | Canonical per-profile observation identity, latest payload, revision, lifecycle, unread state, and archive metadata. Its unique key is profile, source kind, source scope, and external id. | Archived rows are removed 90 days after `archived_at`. Deleting a row cascades to its events and membership claims. |
| `inbox_event` | Significant observation history for an inbox item: classification, transition, summary, detail, and occurrence key. Trivial payload refreshes do not add a row. | The newest 500 rows per item are retained. Older rows are removed first. |
| `feed_membership_claim` | A frontend engine assertion that an item belongs in a profile feed for a source node. | Removed when its item is deleted. Unarchived claims are replaced during synthetic replay; archived claims remain frozen. |
| `event_log` | Append-only transport log used by enabled flow runtimes; their durable offsets are stored separately in `consumer_offset`. | Optional age and per-topic limits are applied by maintenance, while each source topic's newest authoritative snapshot is retained for membership replay. |
| `consumer_offset` | Last ordinary log offset fully committed by a flow. | A monotonic upsert prevents replay from moving a cursor backward. |
| `source_head` | Latest source payload for change detection across producer restarts. | Deleted with a profile purge. |
| `output_command` | Durable, deduplicated action work queue. | Terminal command history is bounded; pending and running work is retained. |
| `node_run` and `activity_event` | Flow diagnostics and the user-facing activity log. | Both are globally bounded diagnostic histories. |

`pipeline.Maintenance` runs retention every five minutes after startup. One
transaction prunes the configured log and diagnostic history, deletes expired
archived items, and trims each item’s event history. The default policy keeps
10,000 node runs, 2,000 terminal output commands, 5,000 activity events,
2,000 terminal jobs, archived items for 90 days, and 500 events per item.

## Observation ingestion

`pipeline.Producer` reloads enabled sources on each tick. A GitHub source uses
`feed.LiveProvider.SourceItems` for cached and conditional GitHub requests,
then passes each observation to `DB.IngestObservation`.

Ingestion performs source-head comparison, classification, item upsert,
optional event append, transport-log append, and source-head update in one
SQLite transaction. An unchanged payload writes nothing. A changed payload
always increments the item revision and updates its payload and source state.
Only a transition or non-trivial attention classification produces an inbox
event. This preserves a current item record without turning harmless refreshes
into user-facing history.

GitHub classification supplies lifecycle and source state. Terminal
transitions archive an item as a system action; reopening restores a
system-archived item. Manual archive state follows the profile’s resurface
policy.

## The `Msg` contract

The event-log transport type is `pipeline.Msg`, re-exported from
`pipelinedb`:

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

`ID` is the decimal event-log offset, `Key` is the stable source identity,
and `Topic` identifies the producing source node. `Ts` is Unix milliseconds.
`Payload` is source-defined JSON; successful source snapshots also carry a
complete current item set. The core fields use their literal Go names when
serialized, so function nodes access `msg.Payload`, `msg.Key`, `msg.ID`, and
`msg.Topic`.

## Flows

Flow definitions live in `flows/*.yaml`; the filename stem is the flow id.
`internal/desktop/pipeline/flow` strictly decodes and validates every file.
The top-level shape is:

```yaml
version: 1
name: Frontend Triage
enabled: true
resurface: state-changes
nodes: []
wires: []
```

Supported node types are:

| Type | Role |
| --- | --- |
| `github-source` | Backend source with `kind`, optional search `query`, and optional `limit`. |
| `github-filter` | Frontend processor that passes or rejects GitHub messages by configured attributes. |
| `function` | Author-provided JavaScript processor with one to sixteen outputs. |
| `feed` | Terminal membership target. The flow-qualified node id is the feed id. |
| `action` | Terminal action target referring to a headless-capable action in `actions.yml`. |

Wires are directed `{from, out?, to}` edges. Validation rejects unknown nodes,
invalid node configuration, invalid ports, duplicate wires, cycles, source
targets, and wires from terminal nodes. A flow’s `.ui.yaml` sibling stores
canvas positions, while its `.sidebar.yaml` sibling stores folder and ordering
metadata for feed nodes.

### Resurface policy

The optional `resurface` field controls whether a manually archived item
returns to the active inbox when later activity arrives. It accepts:

- `state-changes` (the default): only a transition out of a terminal state
  resurfaces a manually archived item; ordinary activity does not.
- `all`: an activity-classified observation can resurface a manually archived
  item.
- `never`: a manual archive is retained even when the source later changes.

System archives return on a transition out of a terminal state. The policy
applies to manual triage, not to an item’s source identity or event history.

## Engine and membership replay

The frontend engine runs processor nodes in a worker and walks the flow as a
DAG. Normal processing reads after the flow’s durable offset. A committed
batch atomically writes feed membership claims, enqueues action commands,
records node metrics, and advances its offset; replaying an already committed
offset is a no-op. `Discard` values are accounting input rather than persisted
rows: their aggregate is reflected in each node run’s drop count. Action
commands are deduplicated by action id and source occurrence key.

Startup and deploy use a different path. The client captures the current
log tail, evaluates each current source node's latest authoritative snapshot
while preserving the source topic that observed each item, and resolves feed
outputs against the current unarchived inbox. `ActivateReplay` then atomically
replaces replayable claims, removes obsolete flow structure, and advances the
consumer to the captured tail. If activation fails, both claims and the prior
consumer offset remain intact, so the last-known-good runtime can continue.
The synthetic replay path cannot enqueue actions. This makes membership
changes deterministic, prevents cross-source claims, and prevents a flow edit
or app restart from repeating side effects.

## Inbox views and triage

The sidebar exposes **Inbox**, **Open**, **Archive**, **All**, and **Unfiled**
views, plus individual feeds. Inbox and Open exclude archived items; Archive
shows archived items; All includes both; Unfiled finds items with no membership
claim. The detail pane shows an item’s retained event history and configured
manual actions.

Triage writes use optimistic revision checks. Users can mark an item unread or
archive/unarchive it; a stale revision fails instead of silently overwriting a
newer observation. Default feed shortcuts are `j`/Down and `k`/Up to move,
`o`/Enter to open in a browser, `u` to toggle the unread filter, `e` to
archive or unarchive, `Shift+U` to mark unread, and `r` to refresh. They can
be changed in Keybinding Settings.

## Actions

`actions.yml` is the global desktop action catalog. Its `launch-session`,
`shell`, and `publish-message` action types are validated before execution.
Both flow outputs and detail-pane invocations use the durable output-command
queue. Background commands retry up to the configured limit; command output
and failure diagnostics are retained with the command record.

## Testing

Go tests under `internal/desktop/pipeline/...` use temporary real SQLite
databases to cover ingestion, classification, membership replay, retention,
and action behavior. Frontend Vitest tests cover the graph engine, node
registries, views, keybindings, and triage state. Docker Playwright tests
exercise the desktop UI against isolated fixtures.

Run the project checks with:

```bash
mise run desktop:test
mise run check
mise run desktop:e2e
```

The e2e task is Docker-only. Do not run desktop integration tests directly on
the host.

## Current architecture and remaining gaps

The inbox-first cutover is landed: Go owns canonical observations and triage;
the frontend owns derived membership and presentation. There is no parallel
legacy read-state path.

Remaining work is intentionally outside this pipeline’s persistence model:

- Engine-driven automatic triage rules are deferred; triage is currently
  source classification plus explicit user action.
- Per-node execution status is recorded after a run, not exposed as a live
  in-flight signal in the canvas.
- Screenshot-style e2e assertions are deferred in favor of behavioral and
  durable-state coverage.

## Key files

| Concern | Path |
| --- | --- |
| Pipeline database and retention | `internal/desktop/pipeline/pipelinedb/` |
| Ingestion and GitHub classification | `internal/desktop/pipeline/producer.go`, `github_classify.go` |
| Flow schema and loader | `internal/desktop/pipeline/flow/` |
| Wails pipeline API | `desktop/pipelineservice.go` |
| Sidebar and triage UI | `desktop/frontend/src/components/SideBar.vue`, `FeedList.vue`, `DetailPane.vue` |
| Frontend graph engine | `desktop/frontend/src/pipeline/engine/` |
| Keybinding catalog | `desktop/frontend/src/keybindings/catalog.ts` |
