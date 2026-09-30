# Canonical inbox item contract

- **Status:** accepted
- **Date:** 2026-07-24

## Context

The pipeline had two GitHub-shaped seams left over from when `github-source`
was the only source: the TS presentation layer (`feedPresentation.ts`) closed
over a github/webhook binary and decoded GitHub payload fields by name, and
the Go action gate (`desktop/pipelineservice.go`) rejected any `InvokeAction`
whose item `SourceKind != "github"`. Both seams already agreed, in practice,
on the same de-facto payload vocabulary — `id`/`title`/`url`/`kind`/`repo`/
`num`/`author`/`body`/`labels`/`state`/`updatedAt` — documented three
half-aligned places (`webhook-source/prompt.ts`'s `FEED_ITEM_SHAPE`, the
webhook conformance probe's five-field subset, and both node help docs) but
never declared as the thing every provider is expected to speak. Adding a
provider meant editing every consumer instead of registering an adapter.

## Decision

Bless the existing key vocabulary as the canonical, provider-neutral inbox
item contract — no new keys, no schema change:

| Key | Meaning | Backing today |
| --- | --- | --- |
| `id` | stable identity within source scope | `external_id` column |
| `title` | heading | `title` column |
| `url` | external link; gates open/copy affordances | `url` column |
| `kind` | provider-defined type label (`PR`, `Issue`, `Alert`, …); defaults to `Item` | payload |
| `repo` | container/context label (repo, channel, dashboard) | payload |
| `num` | short ordinal badge | payload |
| `author` | actor | payload |
| `body` | long text (markdown tolerated) | payload |
| `labels` | string tags | payload |
| `state` | provider state; classifier maps to lifecycle | `source_state`/`lifecycle` |
| `updatedAt` | activity timestamp | `last_event_at` column |

All keys are optional except `id` and `title`. Extra top-level payload fields
(GitHub's `branch`/`prompt`/`reason`) are provider enrichment, decoded only by
that provider's own adapter — there is no `ext.<provider>` namespace, because
namespacing would orphan every already-stored payload the moment it landed.

**Every item has a kind.** A payload that declares no `kind` (or a blank one)
projects as `Item` — `DefaultItemKind` in
`internal/app/dispatch/action_item.go`, `DEFAULT_ITEM_KIND` in
`desktop/frontend/src/lib/itemPresentation.ts`. Without this, an untyped item
was automatable only by an action with no `applies_to` at all: it could not be
named, so it never appeared in the actions editor's autocomplete and
`applies_to: [Item]` matched nothing. Making the default a real kind means
untyped deliveries are a targetable class rather than a dead end — which is
what lets a webhook sender fire into the app before anyone has written a
`function` node to shape its payload. The default is *derived at the decode
boundary*, never written into stored payloads: no migration, and the raw
payload still answers "did the source actually send a kind?" — which is why
the webhook conformance probe keeps listing `kind` as a missing field.

The contract is enforced by two provider-dispatch seams, both now
`sourceKind`-keyed registries instead of hard-coded gates:

- **Presentation** (`desktop/frontend/src/lib/itemPresentation.ts`): the
  canonical projections — kind styling, container/byline lines, snippet,
  search haystack, clipboard text — are module functions applied identically
  to every source. The only thing a provider adapter can vary is
  `sourceLabel`, the badge `mark`, and the detail-pane `actionContextLine`.
- **Actions** (`internal/app/dispatch/action_item.go`,
  `desktop/pipelineservice.go`): `DecodeActionItem` projects any source's
  persisted payload into `{ID, Kind, Payload}` and passes the grab bag through
  unmodified. `ActionApplicability` gates an action on an item by two rules —
  (1) `applies_to` is empty or matches the item's canonical `kind`
  case-insensitively, and (2) every hard payload capability the action's
  templates require is satisfiable. Today the only hard requirement is
  template-derived: a headless launch-session action's `repo_template` must
  render non-blank over the item's payload (`RenderRepoTarget`, shared with
  the executor so the probe can never drift from execution). `ActionViews`
  and `InvokeAction` both key off item id, not `sourceKind` — any source can
  offer and run an action once its kind and payload satisfy it.

Webhook items get a real lifecycle instead of staying permanently active. The
webhook classifier reads the canonical top-level `state` (case-insensitive)
and maps it like GitHub's classifier: `resolved`, `closed`, and `done`
(`webhookTerminalStates` in `internal/app/sources/webhook/webhook_source.go`) are
terminal and system-archive the item on entry, with the state as the archive
reason; any other or absent state keeps the item active, so a stateless
webhook behaves exactly as before. A later delivery that leaves a terminal
state resurfaces the item, mirroring GitHub's reopen handling. A first
delivery that already carries a terminal state is terminal but not
auto-archived, matching GitHub's first-seen behavior.

No payload, function-node, or `actions.yml` migration follows from this
decision — the canonical contract is exactly the key set already in use, so
every stored `inbox_item` payload, function-node transform, and
`applies_to`/`{{ .Payload.* }}` template in `actions.yml` keeps working
byte-for-byte.

Filter nodes remain per-provider by design: `github-filter` stays
GitHub-shaped and no neutral filter node is built. A generic filter would need
to either constrain itself to the canonical fields (losing GitHub-specific
filtering power) or reinvent per-provider branching inside one node — neither
is worth it while GitHub is the only source with a filter node.

## Consequences

- Adding a provider means registering a Go classifier (`SourceAdapter`) and a
  TS `ItemPresentation` adapter; no existing consumer — feed row, detail pane,
  search, clipboard, sidebar summary, flow-editor preview, or the action
  gate — needs to change.
- Any item, regardless of `sourceKind`, can offer and run a detail-pane action
  once its canonical `kind` and payload satisfy the action's requirements.
  GitHub items with zero applicable actions no longer show empty ACTIONS
  scaffolding — this was a sanctioned behavior change, not a regression.
- Webhook senders that want archive/resurface behavior only need to add
  `state` to their payload; senders that never send it keep today's
  manual-triage-only behavior with no code change on either side.
- Executors now see the full stored payload (grab bag included) instead of a
  GitHub-`feed.Item`-normalized projection. Canonical keys are unchanged for
  GitHub items; a webhook payload lacking a template-referenced key still
  fails the run with a visible error, exactly as flow-driven action nodes do
  today.
- The conformance probe (`MissingFeedItemFields`) intentionally stays
  narrower than the full contract — see
  `internal/app/sources/webhook/webhook_source.go`'s `feedItemFields` comment.
