# The desktop source pipeline

Hive Desktop is an inbox-first system for GitHub observations. Each profile is
a flow. Sources collect observations, ingestion classifies and durably persists
them, and the flow engine decides which unarchived items belong to each sidebar
feed. This separation keeps an item’s identity and triage state stable while
flows, filters, and feed membership change.

## Architecture

The pipeline has three cooperating parts:

1. **Go ingestion** polls configured pull-mode source nodes. For every changed
   observation it classifies the change and unconditionally updates the
   corresponding `inbox_item`; noteworthy classifications also append an
   `inbox_event`. Ingestion owns item identity, payload, revision, lifecycle,
   unread state, and archive state.
2. **The flow engine** (`cmd/desktop/internal/app/runtime`) evaluates each enabled flow.
   Its ordinary event-log pass advances a durable consumer offset, records
   node-run diagnostics, and can enqueue deduplicated actions. On startup and
   deploy, its synthetic replay evaluates each source node's latest
   authoritative snapshot and resolves outputs against the current unarchived
   inbox. It then atomically installs those membership claims and fast-forwards
   the consumer to the captured log tail without enqueuing actions.
3. **The desktop UI** reads inbox views and feed membership claims. It provides
   triage controls over the same durable inbox state rather than maintaining a
   separate read-state store.

```
GitHub API
   │
   ▼
sources.github → Go producer → classify and persist
                               │
                 ┌─────────────┴─────────────┐
                 ▼                           ▼
            inbox_item                   inbox_event
                 │                           │
                 ▼                           ▼
         flow engine                   observed history
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
`cmd/desktop/internal/app/data`. It is separate from `hive.db` so
pipeline polling and desktop interactions do not compete with CLI/TUI writes.
All timestamps stored by this database are Unix milliseconds.

Every subsystem below is owned and wired by `app.New`, started by `App.Start`
and unwound by `App.Close` — `package main` holds none of it. The Wails
services in `cmd/desktop/internal/adapter/wailsui/` are transport over the per-domain
services in `cmd/desktop/internal/app/`.

| Table | Purpose | Retention / cascade behavior |
| --- | --- | --- |
| `inbox_item` | Canonical per-profile observation identity, latest payload, revision, lifecycle, unread state, and archive metadata. Its unique key is profile, source kind, source scope, and external id. | Archived rows are removed 90 days after `archived_at`. Deleting a row cascades to its events and membership claims. |
| `inbox_event` | Significant observation history for an inbox item: classification, transition, summary, detail, and occurrence key. Trivial payload refreshes do not add a row. | The newest 500 rows per item are retained. Older rows are removed first. |
| `feed_membership_claim` | A flow engine assertion that an item belongs in a profile feed for a source node. | Removed when its item is deleted. Unarchived claims are replaced during synthetic replay; archived claims remain frozen. |
| `event_log` | Append-only transport log used by enabled flow runtimes; their durable offsets are stored separately in `consumer_offset`. | Optional age and per-topic-row limits are applied by maintenance; the newest snapshots per topic are retained by their own bound (default 3) since older full-source snapshots are superseded, and each topic's newest authoritative snapshot is always kept for membership replay. |
| `consumer_offset` | Last ordinary log offset fully committed by a flow. | A monotonic upsert prevents replay from moving a cursor backward. |
| `source_head` | Latest source payload for change detection across producer restarts. | Deleted with a profile purge. |
| `webhook_capture` | Most recent request body per webhook source topic, for the node editor's preview/prompt affordances. | One row per topic, replaced on every delivery. |
| `output_command` | Durable, deduplicated action work queue, shared by `action` nodes (keyed by their `actions.yml` id) and `notify` nodes (keyed by a synthetic `notify:<flowId>/<nodeId>` id). | Terminal command history is bounded; pending and running work is retained. |
| `node_run` and `activity_event` | Flow diagnostics and the user-facing activity log. | Both are globally bounded diagnostic histories. |

`pipeline.Maintenance` runs retention every five minutes after startup. One
transaction prunes the configured log and diagnostic history, deletes expired
archived items, and trims each item’s event history. The default policy keeps
10,000 node runs, 2,000 terminal output commands, 5,000 activity events,
2,000 terminal jobs, archived items for 90 days, 500 events per item, and the
newest 3 source snapshots per topic.

After opening and migrating the pipeline store, but before sharing it with any
writer, startup checks SQLite's freelist. It runs a full `VACUUM` only when at
least 20 percent and 16 MiB of the file are reclaimable, then checkpoints the
compact image from WAL into the main file. This maintenance is best-effort:
measurement or compaction failures are logged and startup continues.

## Observation ingestion

`pipeline.Producer` reloads enabled sources on each tick. A GitHub source uses
`feed.LiveProvider.SourceItems` for cached and conditional GitHub requests,
then passes each observation to `DB.IngestObservation`.

An instance may declare a `MinInterval` — the exec source's `interval` field is
the only one that does today — and the tick skips it until that floor expires.
There is still one ticker, so the floor is quantized to it, and a skipped source
produces nothing at all rather than an empty snapshot. `Producer.Refresh`, what
a manual refresh calls, ignores every floor. A source whose `Produce` fails is
recorded in Activity as `RefreshFailed`, at most once an hour while it stays
broken (ADR a-command-is-a-source).

Ingestion performs source-head comparison, classification, item upsert,
optional event append, transport-log append, and source-head update in one
SQLite transaction. An unchanged payload writes nothing. A changed payload
always increments the item revision and updates its payload and source state.
Only a transition or non-trivial attention classification produces an inbox
event. This preserves a current item record without turning harmless refreshes
into user-facing history.

Source classifiers supply lifecycle and source state (docs/decisions/0008).
Terminal transitions archive an item as a system action; reopening restores a
system-archived item. Manual archive state follows the profile’s resurface
policy.

### Webhook ingress

`sources.webhook` nodes are push-driven and bypass the producer entirely
(docs/decisions/0007, 0014, and 0019). The listener shares one loopback `http`
server with the agent API, on by default (ADR agent-http-api). When `http.enabled` is true
it binds `http.host` (loopback-only) and `http.port`; port `0` asks the OS to
select the port directly, and the running endpoint reports the selected address.
Typed process overrides are `HIVE_DESKTOP_HTTP_ENABLED`,
`HIVE_DESKTOP_HTTP_HOST`, and `HIVE_DESKTOP_HTTP_PORT`. Settings → Integrations →
Webhooks edits the persisted values. The listener resolves `/hooks/<path>` routes per request
against the push-mode connector instances `ingest.Resolver` builds from the
current flow set — the same resolution the producer's pull sources go through,
so enabled/disabled filtering happens once for both. A delivery calls `IngestObservation` under topic
`source:<flowId>/<nodeId>` with source kind `webhook` and scope `<nodeId>`:
a top-level `id` is the stable key (else the body's SHA-256, deduplicating
exact duplicate deliveries), and `title`/`url` are promoted for feed
rendering — the rest of the canonical item contract (docs/decisions/0008)
comes from the payload as-is. The webhook classifier maps the canonical
top-level `state` to lifecycle exactly like GitHub: `resolved`, `closed`, and
`done` (case-insensitive) are terminal and system-archive the item; any other
or absent state keeps it active, and a later delivery that leaves a terminal
state resurfaces it. After each write the listener appends the topic's
complete unarchived item set as the authoritative snapshot, so membership
replay treats webhook sources exactly like polled ones. The last request body
per topic is kept in `webhook_capture` for the node editor's payload preview,
feed-shape hint, and LLM transform prompt.

### Command sources

`sources.exec` nodes run the user's own command through `sh -c` on the tick, in
the environment `execenv` resolves, and ingest stdout as an authoritative
snapshot: a JSON array of objects, one message per object, keyed by its
top-level `id` under source kind `exec` and scope `<nodeId>`. The item contract
is the webhook one — the classifier is the same code (`sources/canonical`) — so
`title`/`url` promotion and the `state` lifecycle behave identically. What the
webhook cannot have, this does: absence is authoritative, so an item that leaves
the snapshot is confirmed `resolved` and archived, as with Grafana alerts.

A non-zero exit, a timeout, or stdout that is not a complete JSON array of
identified items fails the run — before anything is emitted — so a broken
command leaves the previous snapshot in place instead of archiving everything
the source owns. `[]` is the one way to say the source is genuinely empty.

## The `Msg` contract

The event-log transport type is `ingest.Msg`, an alias for `models.Msg`:

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
`cmd/desktop/internal/app/flow` strictly decodes and validates every file.
The top-level shape is:

```yaml
version: 1
name: Frontend Triage
enabled: true
resurface: state-changes
nodes: []
wires: []
```

Source node types are namespaced `sources.<name>` and come from the connector
registry (`cmd/desktop/internal/app/sources`, ADR source-connector-registry) rather than being listed in the
flow package; the rest are declared in `flow` directly. Supported node types
are:

| Type | Role |
| --- | --- |
| `sources.github` | Backend source with `kind`, optional search `query`, and optional `limit`. |
| `sources.webhook` | Backend source served by the local webhook listener: JSON POSTed to `/hooks/<path>` becomes this node's messages. Optional per-node `secret` (X-Hive-Secret header). |
| `github-filter` | Processor that passes or rejects GitHub messages by configured attributes. |
| `function` | Author-provided JavaScript processor with one to sixteen outputs. |
| `feed` | Terminal membership target. The flow-qualified node id is the feed id. |
| `action` | Terminal action target referring to a headless-capable action in `actions.yml`. |
| `notify` | Terminal system-notification target with inline `title`/`body` templates, optional `severity` and `sound`. Delivery respects the app's notification settings. |

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

The engine runs in Go (`cmd/desktop/internal/app/runtime`, ADRs goja-script-runtime and flow-engine-in-go) and walks
the flow as a DAG, evaluating `function` nodes through goja. `runtime.Engine`
drives it: it installs a runner per enabled flow at startup, reinstalls them
when the flow set changes, and drains on every append. Nothing about execution
depends on a window being open. Normal processing reads after the flow’s durable offset. A committed
batch atomically writes feed membership claims, enqueues action commands,
records node metrics, and advances its offset; replaying an already committed
offset is a no-op. A feed output whose inbox row is written under the empty
pre-#63 scope is healed onto its account scope during the commit; one whose key
resolves to no row at all is minted from the payload it carried — the row behind
a per-entity item a `function` node split out under a key that never went
through ingest (ADR function-node-per-entity-feed-items). A key-less output has no identity to mint under and
is skipped and logged rather than failing the batch, so no unresolvable item can
wedge the consumer at one offset. `Discard` values are accounting input rather than persisted
rows: their aggregate is reflected in each node run’s drop count. Action
commands are deduplicated by action id and source occurrence key.

Startup and deploy use a different path. The engine captures the current
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
`shell`, `publish-message`, and `clipboard` action types are validated before
execution. Both flow outputs and detail-pane invocations use the durable
output-command queue, except `clipboard`, which renders without one (ADR clipboard-action-type).
Background commands retry up to the configured limit; command output
and failure diagnostics are retained with the command record.

A `launch-session` action can target a rendered repository or a literal agent
workspace directory. Either fixed target runs headlessly; with neither, manual
invocation asks for one. Workspace launches use the current workspace command
and refuse one that drops the rendered prompt or does not shell-quote it
(ADR a-launch-session-action-targets-either-a-repository-or-an-agent-workspace).

An action may declare `inputs` — values collected from the user when it is
invoked and rendered into its templates as `.Inputs.<name>` (ADR action-declared-inputs). An
action with a required input that has no default cannot run headlessly, so a
flow `action` node may not reference it.

## Testing

Go tests under `cmd/desktop/internal/app/...` use temporary real SQLite databases to
cover ingestion, classification, membership replay, retention, and action
behavior. Frontend Vitest tests cover the flow editor, node
registries, views, keybindings, and triage state. Docker Playwright tests
exercise the desktop UI against isolated fixtures.

The engine's own fixtures are `cmd/desktop/internal/app/runtime/testdata/parity/*.json`: a
flow, a batch, and the exact `CommitBatch` it is worth. They began as the proof
that the port off the browser engine was faithful — both engines executed them
and had to agree — and remain the place a change to routing, sink tagging or
node-run accounting belongs.

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
| Pipeline database and retention | `cmd/desktop/internal/app/data/` |
| Ingestion and source classification | `cmd/desktop/internal/app/ingest/producer.go`, `cmd/desktop/internal/app/sources/github/classify.go`, `cmd/desktop/internal/app/sources/canonical/` (the shared user-shaped-payload contract), `cmd/desktop/internal/app/sources/webhook/`, `cmd/desktop/internal/app/sources/exec/` |
| Flow schema and loader | `cmd/desktop/internal/app/flow/` |
| Output-command dispatch and executors | `cmd/desktop/internal/app/dispatch/` |
| Inbox orchestration | `cmd/desktop/internal/app/inbox_service.go` |
| Wails pipeline API | `cmd/desktop/internal/adapter/wailsui/pipelineservice.go` |
| Subsystem wiring and lifecycle | `cmd/desktop/internal/app/app.go` |
| Sidebar and triage UI | `cmd/desktop/frontend/src/components/SideBar.vue`, `FeedList.vue`, `DetailPane.vue` |
| Flow engine | `cmd/desktop/internal/app/runtime/` |
| Flow editor | `cmd/desktop/frontend/src/pipeline/` |
| Keybinding catalog | `cmd/desktop/frontend/src/keybindings/catalog.ts` |
