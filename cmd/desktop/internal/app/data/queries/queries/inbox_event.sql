-- name: ListInboxEventsByItem :many
SELECT * FROM inbox_event WHERE item_id = ? ORDER BY id DESC LIMIT ?;

-- name: InsertInboxEvent :one
INSERT INTO inbox_event (item_id, kind, transition, attention, occurrence_key, summary, detail, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (item_id, occurrence_key) WHERE occurrence_key IS NOT NULL DO NOTHING
RETURNING *;
