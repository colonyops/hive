# A session can link to many inbox items

- **Status:** accepted
- **Date:** 2026-09-17

## Context

Multi-select combines several inbox items into one session, and that session
must stay visible from every selected item's detail pane. A link table keyed
on the session id alone cannot represent that.

## Decision

`item_session` is a many-to-many association in `desktop-pipeline.db`. Its
primary key is the session id plus the item's durable coordinates
(`profile_id`, `source_kind`, `source_scope`, `external_id`). Agent workspace
chats link to items the same way, in `item_chat`.

- **The link lives in desktop state, not in `hive.db`.** Hive does not know
  what an inbox item is.
- **The link names the item by its coordinates, not `inbox_item.id`, and has
  no foreign key.** Those four columns are `inbox_item`'s own `UNIQUE` key.
  `ActivateReplay` deletes and rebuilds a profile's rows when its graph
  changes, so a row id or a foreign key would drop every link on an ordinary
  flow edit. Deleting a profile drops its links, and a `source_scope` heal
  rescopes them with the row.
- **Nothing about the session is copied.** Name, repo, state and liveness are
  read from hive on every read.
- **The read reconciles.** `SessionsService.ItemSessions` drops links that a
  successful hive listing cannot account for, and drops nothing when the
  listing fails.

A multi-item create request carries an ordered list of inbox row ids
(`itemIds`). Go resolves them, drops duplicates and items that no longer
exist, and sends one launch request with every unique origin. The launcher
records the one resulting session against each origin.

Persisted retry metadata stores the plural ids. Its decoder still accepts
the singular `itemId`, so a failed request saved by an older build remains
retryable.

## Consequences

- One session appears in several item detail panes without duplicating the
  hive session.
- Linking the same session and item again is idempotent.
- An item that disappears before submit does not block creation; the
  surviving origins still get links.
