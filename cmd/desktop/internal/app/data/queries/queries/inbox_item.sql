-- name: InsertInboxItem :one
-- Seed-only plain insert for deterministic desktop fixtures. The ingestion
-- path receives its classify-and-upsert contract in a later phase.
INSERT INTO inbox_item (
    profile_id, source_kind, source_scope, external_id,
    title, url, payload, revision, unread,
    lifecycle, first_seen_at, last_event_at
) VALUES (
    ?, ?, ?, ?,
    ?, ?, ?, 1, ?,
    ?, ?, ?
)
RETURNING *;

-- name: UpsertInboxItem :one
INSERT INTO inbox_item (
    profile_id, source_kind, source_scope, external_id,
    title, url, payload, revision, unread,
    archived_at, archived_actor, archived_reason,
    lifecycle, source_state, first_seen_at, last_event_at
) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (profile_id, source_kind, source_scope, external_id) DO UPDATE SET
    title = excluded.title, url = excluded.url, payload = excluded.payload,
    revision = inbox_item.revision + 1, unread = excluded.unread,
    archived_at = excluded.archived_at, archived_actor = excluded.archived_actor,
    archived_reason = excluded.archived_reason, lifecycle = excluded.lifecycle,
    source_state = excluded.source_state, last_event_at = excluded.last_event_at
RETURNING *;

-- name: GetInboxItemByExternalID :one
SELECT * FROM inbox_item
WHERE profile_id = ? AND source_kind = ? AND source_scope = ? AND external_id = ?;

-- name: GetInboxItemIDByExternalID :one
SELECT id FROM inbox_item
WHERE profile_id = ? AND source_kind = ? AND source_scope = ? AND external_id = ?;

-- name: GetInboxItemByID :one
SELECT * FROM inbox_item WHERE id = ?;

-- Every user-facing feed view is claim-scoped: source observations that did
-- not reach a feed terminal remain internal pipeline data and never leak into
-- workspace categories. Trash is the only exception: it deliberately surfaces
-- unrouted and ignored items so users can debug rules and un-ignore.
-- name: GetUnarchivedInboxItemByID :one
SELECT * FROM inbox_item WHERE id = ? AND profile_id = ? AND archived_at IS NULL;

-- name: RescopeInboxItem :exec
-- Move a row to a new source_scope. Used to heal pre-#63 rows written with an
-- empty scope onto the account-scoped identity every post-#63 read keys on
-- (issue #95); the caller only rescopes when the target identity is free, so
-- this never collides with the UNIQUE (profile_id, source_kind, source_scope,
-- external_id) index.
UPDATE inbox_item SET source_scope = sqlc.arg(source_scope) WHERE id = sqlc.arg(id);

-- name: FindInboxItemsByExternalID :many
SELECT * FROM inbox_item
WHERE external_id = sqlc.arg(external_id)
  AND (sqlc.arg(profile_id) = '' OR profile_id = sqlc.arg(profile_id))
ORDER BY profile_id, source_kind, source_scope;

-- name: ListAllInboxItems :many
SELECT * FROM inbox_item
WHERE (sqlc.arg(profile_id) = '' OR profile_id = sqlc.arg(profile_id))
ORDER BY last_event_at DESC, id DESC
LIMIT sqlc.arg(lim);

-- name: ListUnarchivedInboxItemsByProfile :many
SELECT * FROM inbox_item WHERE profile_id = ? AND archived_at IS NULL;

-- name: ListInboxItemsByFeed :many
SELECT DISTINCT i.* FROM inbox_item i
JOIN feed_membership_claim c ON c.item_id = i.id
WHERE c.profile_id = ? AND c.feed_id = ? AND i.archived_at IS NULL AND i.ignored_at IS NULL
ORDER BY i.last_event_at DESC, i.id DESC LIMIT ?;

-- Archived items stay visible inside the feeds they matched, demoted below
-- active rows and lazy-loaded when the archived section expands.
-- name: ListArchivedInboxItemsByFeed :many
SELECT DISTINCT i.* FROM inbox_item i
JOIN feed_membership_claim c ON c.item_id = i.id
WHERE c.profile_id = ? AND c.feed_id = ? AND i.archived_at IS NOT NULL AND i.ignored_at IS NULL
ORDER BY i.archived_at DESC, i.id DESC LIMIT ?;

-- Trash holds items with no path to any feed (routing produced no claims)
-- plus user-ignored items. It is a utility/debug surface, never a queue.
-- name: ListInboxItemsTrash :many
SELECT i.* FROM inbox_item i
WHERE i.profile_id = ?
  AND (i.ignored_at IS NOT NULL
       OR NOT EXISTS (SELECT 1 FROM feed_membership_claim c WHERE c.profile_id = i.profile_id AND c.item_id = i.id))
ORDER BY i.last_event_at DESC, i.id DESC LIMIT ?;

-- name: CountInboxItemsByFeed :many
SELECT
    c.feed_id AS feed_id,
    CAST(COUNT(DISTINCT CASE WHEN i.archived_at IS NULL THEN i.id END) AS INTEGER) AS total,
    CAST(COUNT(DISTINCT CASE WHEN i.archived_at IS NULL AND i.unread = 1 THEN i.id END) AS INTEGER) AS unread,
    CAST(COUNT(DISTINCT CASE WHEN i.archived_at IS NOT NULL THEN i.id END) AS INTEGER) AS archived
FROM feed_membership_claim c
JOIN inbox_item i ON i.id = c.item_id
WHERE c.profile_id = ? AND i.ignored_at IS NULL
GROUP BY c.feed_id;

-- name: SetInboxItemUnread :one
UPDATE inbox_item SET unread = ?, revision = revision + 1
WHERE id = ? AND revision = ?
RETURNING *;

-- Bulk read-state clear, claim-scoped so it covers exactly the rows
-- CountInboxItemsByFeed reports as unread. Deliberately NOT revision-guarded:
-- a bulk triage has no single row whose revision the caller could have read,
-- and read state is low-stakes. The revision still advances, so an optimistic
-- per-item write holding a pre-clear copy is rejected and re-reads instead of
-- silently resurrecting the unread flag. Archived and ignored rows are left
-- alone: neither is in the active list this clears.
-- (Keep this file ASCII-only. sqlc mixes byte and rune offsets when it edits a
-- query, so a multi-byte character anywhere above corrupts a later query.)
-- name: MarkFeedInboxItemsRead :execrows
UPDATE inbox_item SET unread = 0, revision = revision + 1
WHERE unread = 1 AND archived_at IS NULL AND ignored_at IS NULL
  AND id IN (SELECT c.item_id FROM feed_membership_claim c WHERE c.profile_id = ? AND c.feed_id = ?);

-- The workspace-wide variant: every feed in the profile, same scoping rules.
-- Unrouted observations stay untouched; they live only in Trash, which carries
-- no unread semantics.
-- name: MarkProfileInboxItemsRead :execrows
UPDATE inbox_item SET unread = 0, revision = revision + 1
WHERE unread = 1 AND archived_at IS NULL AND ignored_at IS NULL
  AND id IN (SELECT c.item_id FROM feed_membership_claim c WHERE c.profile_id = ?);

-- name: ToggleInboxItemArchived :one
UPDATE inbox_item SET
    ignored_at = CASE WHEN archived_at IS NULL THEN NULL ELSE ignored_at END,
    archived_at = CASE WHEN archived_at IS NULL THEN ? ELSE NULL END,
    archived_actor = CASE WHEN archived_at IS NULL THEN 'manual' ELSE NULL END,
    archived_reason = CASE WHEN archived_at IS NULL THEN 'manual' ELSE NULL END,
    revision = revision + 1
WHERE id = ? AND revision = ?
RETURNING *;

-- name: ToggleInboxItemIgnored :one
UPDATE inbox_item SET
    ignored_at = CASE WHEN ignored_at IS NULL THEN ? ELSE NULL END,
    unread = CASE WHEN ignored_at IS NULL THEN 0 ELSE 1 END,
    archived_at = CASE WHEN ignored_at IS NULL THEN NULL ELSE archived_at END,
    archived_actor = CASE WHEN ignored_at IS NULL THEN NULL ELSE archived_actor END,
    archived_reason = CASE WHEN ignored_at IS NULL THEN NULL ELSE archived_reason END,
    revision = revision + 1
WHERE id = ? AND revision = ?
RETURNING *;

-- name: DeleteInboxItemsByProfile :exec
DELETE FROM inbox_item WHERE profile_id = ?;

-- name: PruneArchivedInboxItems :exec
-- Cascades to inbox_event and feed_membership_claim through their item FKs.
DELETE FROM inbox_item WHERE archived_at IS NOT NULL AND archived_at <= ?;

-- name: ListUnarchivedInboxItemsBySource :many
-- The complete current item set of one source identity, oldest activity
-- first: the webhook listener builds its per-delivery authoritative
-- snapshot from these rows.
SELECT * FROM inbox_item
WHERE profile_id = ? AND source_kind = ? AND source_scope = ? AND archived_at IS NULL
ORDER BY last_event_at ASC, id ASC;
