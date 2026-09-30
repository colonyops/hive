# An item↔session link is desktop state, keyed on the item's coordinates

- **Status:** superseded by [A session can link to many inbox items](2026-09-17-a-session-can-link-to-many-inbox-items.md)
- **Date:** 2026-08-02

## Context

A `launch-session` action creates a hive session and then forgets it. The
outcome is written to `output_command.result_json`, but nothing can be asked
"which sessions did this item start?" — the durable command's dedup key is the
message's *occurrence key*, not the item's external id, so a command that
outlives a restart has no way back to the row that produced it. From the item's
side there was nothing at all.

Two places could hold the association. Hive's own session record has `Tags` and
`Metadata`, and stamping the item there would travel with the session and show
up in the hive CLI. Or the desktop keeps it in `desktop-pipeline.db`, which is
already where inbox items live.

## Decision

1. **The association is a `item_session` table in `desktop-pipeline.db`**, keyed
   by hive's session id. Hive is a separate product behind an anti-corruption
   layer; teaching its shared `hive.db` what an inbox item is merges two models
   the seam exists to keep apart, and "which items have sessions?" would become
   a scan of every session with string parsing on top. The item's id is also
   stamped on the session as a hive **tag**, because that is what hive's tags
   are for — but it is presentational and never read back. One authority.

2. **The link names the item by its coordinates**
   (`profile_id`, `source_kind`, `source_scope`, `external_id`), not by
   `inbox_item.id`, and carries no foreign key. Those four are `inbox_item`'s
   own `UNIQUE` key. The row id is not durable enough to hang an association
   on: `ActivateReplay` deletes and rebuilds every row a profile owns whenever
   its graph changes, so a foreign key would silently drop every association on
   an ordinary flow edit. Two consequences follow and are enforced —
   `PurgeProfile` drops the profile's links, and `resolveInboxItemScoped`'s
   in-place `source_scope` heal moves the links with the row.

3. **Nothing about the session is copied.** Name, slug, repo, state and
   liveness are read from hive on every read, so the pane cannot show a name a
   rename has moved on from. Only the link's own `created_at` is the desktop's.

4. **The read is what reconciles.** Nothing tells this app when a session is
   deleted from the CLI, so `SessionsService.ItemSessions` drops links a
   *successful* hive listing could not account for. Behind a listing that
   failed it drops nothing — an unreadable `hive.db` is not evidence a session
   is gone. This mirrors the credential index (ADR credential-store), where `Get` is what
   heals a stale ref.

5. **The item an action ran against is columns on `output_command`.** The notify
   sink already carries the same four fields, inside a payload envelope it mints
   itself; an action's payload *is* the item and cannot be wrapped without
   breaking every template, so they become columns instead. `actionSinks` now
   carries the source identity for the same reason `notifySinks` does. Claiming
   a queued command keeps the routed origin and takes the caller's only when it
   has none — the four columns move as a unit, because a mix of one row's
   profile and another's external id is a reference to no item at all.

6. **One item has many sessions, newest first.** A re-run appends rather than
   replacing: the earlier session still exists, still holds work, and hiding it
   would be a lie about what the item produced.

## Consequences

- An item shows its sessions after a restart, after a replay, and after the
  session is renamed — the three ways the naive designs break.
- A session created by the hive CLI is not linked to anything. That is correct:
  this records what *this app* started on an item's behalf.
- Status is pulled on demand (item selection, and `jobs:updated`, which is when
  a session can have been created, deleted or recycled) rather than polled.
  `HiveSessionManager.RunningSessions` asks about the named sessions only, so an
  item with one session does not pay a tmux round trip per session in the
  install.
- A session whose item was purged keeps running; only the link goes.
- `newSessionsService` takes a `sessionsDeps` struct. Ten positional
  dependencies had stopped saying which `nil` was which.
