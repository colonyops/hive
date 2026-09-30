-- name: GetSourceHeadPayload :one
SELECT payload FROM source_head
WHERE topic = ? AND key = ?;

-- name: UpsertSourceHead :exec
INSERT INTO source_head (topic, key, payload)
VALUES (?, ?, ?)
ON CONFLICT(topic, key) DO UPDATE SET payload = excluded.payload;

-- name: DeleteSourceHead :exec
DELETE FROM source_head WHERE topic = ? AND key = ?;

-- name: ListActiveSourceHeadKeys :many
SELECT h.key
FROM source_head h
JOIN inbox_item i
  ON i.external_id = h.key
 AND i.profile_id = sqlc.arg(profile_id)
 AND i.source_kind = sqlc.arg(source_kind)
 AND i.source_scope = sqlc.arg(source_scope)
WHERE h.topic = sqlc.arg(topic)
  AND i.archived_at IS NULL;

-- name: DeleteSourceHeadByTopicPrefix :exec
DELETE FROM source_head WHERE topic LIKE ? ESCAPE '\';

-- name: DeleteOrphanedSourceHeads :exec
DELETE FROM source_head
WHERE key NOT IN (SELECT external_id FROM inbox_item);
