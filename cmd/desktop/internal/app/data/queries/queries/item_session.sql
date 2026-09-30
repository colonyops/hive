-- name: LinkItemSession :exec
-- A session/item pair is idempotent; one session can link to several items.
INSERT INTO item_session (session_id, profile_id, source_kind, source_scope, external_id, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (session_id, profile_id, source_kind, source_scope, external_id) DO NOTHING;

-- name: ListItemSessions :many
-- Newest first: a re-run appends a session rather than replacing one, so the
-- most recent attempt leads.
SELECT * FROM item_session
WHERE profile_id = ? AND source_kind = ? AND source_scope = ? AND external_id = ?
ORDER BY created_at DESC, session_id DESC;

-- name: DeleteItemSession :exec
DELETE FROM item_session WHERE session_id = ?;

-- name: DeleteItemSessionsByProfile :exec
DELETE FROM item_session WHERE profile_id = ?;

-- name: RescopeItemSessions :exec
-- Follows an inbox row whose source_scope was healed (see
-- resolveInboxItemScoped), so links keyed on the old scope stay reachable.
-- A target link may already exist now that one session can link to many items.
UPDATE OR IGNORE item_session SET source_scope = sqlc.arg(source_scope)
WHERE profile_id = sqlc.arg(profile_id)
  AND source_kind = sqlc.arg(source_kind)
  AND source_scope = ''
  AND external_id = sqlc.arg(external_id);

-- name: DeleteUnscopedItemSessions :exec
-- Removes old rows that RescopeItemSessions skipped because the target
-- session-item association already existed.
DELETE FROM item_session
WHERE profile_id = sqlc.arg(profile_id)
  AND source_kind = sqlc.arg(source_kind)
  AND source_scope = ''
  AND external_id = sqlc.arg(external_id);
