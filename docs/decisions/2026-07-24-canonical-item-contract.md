# Canonical inbox item contract

- **Status:** accepted
- **Date:** 2026-07-24

## Context

Every inbox consumer (feed row, detail pane, search, clipboard, action gate) needs to read items from any
source. Without a declared payload vocabulary, adding a provider means editing every consumer instead of
registering an adapter.

## Decision

The inbox item payload has one provider-neutral key vocabulary:

| Key | Meaning | Backing |
| --- | --- | --- |
| `id` | stable identity within source scope | `external_id` column |
| `title` | heading | `title` column |
| `url` | external link; gates open/copy affordances | `url` column |
| `kind` | provider-defined type label (`PR`, `Issue`, `Alert`, ...); defaults to `Item` | payload |
| `repo` | container/context label (repo, channel, dashboard) | payload |
| `num` | short ordinal badge | payload |
| `author` | actor | payload |
| `body` | long text (markdown tolerated) | payload |
| `labels` | string tags | payload |
| `state` | provider state; the classifier maps it to lifecycle | `source_state`/`lifecycle` |
| `updatedAt` | activity timestamp | `last_event_at` column |

All keys are optional except `id` and `title`. Extra top-level fields are provider enrichment, read only by
that provider's own adapter. There is no `ext.<provider>` namespace.

**Every item has a kind.** A payload with no or blank `kind` projects as `Item` (`DefaultItemKind` in
`cmd/desktop/internal/app/dispatch/action_item.go`, `DEFAULT_ITEM_KIND` in
`cmd/desktop/frontend/src/lib/itemPresentation.ts`; the two must match), so untyped deliveries are
targetable by `applies_to: [Item]`. The default is derived at the decode boundary and never written into
stored payloads.

Two seams consume the contract, both keyed by `sourceKind` registries instead of hard-coded gates:

- **Presentation** (`itemPresentation.ts`): kind styling, byline, snippet, search haystack, and clipboard
  text are canonical projections shared by every source. A provider's `ItemPresentation` varies only
  `sourceLabel`, the badge `mark`, and `markImage`.
- **Actions** (`dispatch/action_item.go`): `DecodeActionItem` projects any persisted payload into
  `{ID, Kind, Payload}` and passes the rest through. `ActionApplicability` allows an action when
  `applies_to` is empty or matches the canonical `kind` case-insensitively, and every payload capability
  its templates require is satisfiable (a headless launch-session's `repo_template` must render non-blank,
  through the same `RenderRepoTarget` the executor uses).

**`state` drives lifecycle for user-shaped payloads.** The shared `sources/canonical` classifier, used by
connectors whose payload is the canonical shape (webhook, exec), treats `resolved`, `closed`, and `done` (case-insensitive) as terminal and
system-archives the item on entry with the state as the reason. Any other or absent state stays active. A
later delivery that leaves a terminal state resurfaces the item. A first delivery already in a terminal
state is terminal but not auto-archived, matching GitHub.

Filter nodes stay per-provider: `github-filter` is GitHub-shaped and there is no neutral filter node.

## Consequences

- Adding a provider means registering a connector (ADR source-connector-registry) and an `ItemPresentation`
  adapter; no existing consumer changes.
- Executors see the full stored payload. A payload missing a template-referenced key fails the run with a
  visible error.
- The webhook conformance probe (`MissingFeedItemFields`) checks only the render-critical subset
  (`id`, `kind`, `repo`, `title`, `url`) and reads the raw payload, so it still reports a missing `kind`.
