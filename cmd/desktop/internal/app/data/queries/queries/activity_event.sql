-- name: AppendActivityEvent :one
-- Append one audit-log row and return it (with its assigned id) so the caller
-- can echo the stored event straight back to subscribers.
INSERT INTO activity_event (created_at, category, severity, title, body, source, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListActivityEvents :many
-- Newest first, paged by a descending id cursor: pass a sentinel above the
-- largest id for the first page, then the smallest id returned to continue.
SELECT * FROM activity_event
WHERE id < ?
ORDER BY id DESC
LIMIT ?;

-- name: PruneActivityEvents :exec
-- Retain only the newest rows globally; bounded diagnostic history like
-- node_run. The id primary key both orders and breaks ties exactly.
DELETE FROM activity_event
WHERE id IN (
    SELECT id FROM activity_event
    ORDER BY id DESC
    LIMIT -1 OFFSET ?
);
