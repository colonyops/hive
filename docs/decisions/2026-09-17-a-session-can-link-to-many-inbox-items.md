# A session can link to many inbox items

- **Status:** accepted
- **Date:** 2026-09-17

## Context

The inbox originally created a session from one item, so `item_session` used
`session_id` as its primary key. Multi-select combines several inbox items into
one session. That session must remain visible from every selected item's detail
pane, which the original key cannot represent.

## Decision

`item_session` is a many-to-many association. Its primary key is the session id
plus the item's durable coordinates (`profile_id`, `source_kind`,
`source_scope`, `external_id`). The coordinates, reconciliation behavior, and
separation from hive state remain as specified by the superseded ADR.

A multi-item create request carries an ordered list of inbox row ids. Go
resolves those ids, generates one combined prompt, derives a repository only
when every item agrees, and sends one launch request with all unique origins.
The launcher records the one resulting session against each origin.

Persisted retry metadata stores the plural ids. Its decoder continues to accept
the former singular `itemId` field so failed requests from an older app version
remain usable.

## Consequences

- One session can appear in several item detail panes without duplicating the
  hive session.
- Repeating the same session-item link remains idempotent.
- The schema migration rebuilds the table and preserves existing links.
- Session creation can continue if a selected item disappears before submit;
  the surviving origins still receive links.
