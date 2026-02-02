-- name: ListSessions :many
SELECT * FROM sessions
ORDER BY created_at DESC;

-- name: GetSession :one
SELECT * FROM sessions
WHERE id = ?;

-- name: SaveSession :exec
INSERT INTO sessions (
    id, name, slug, path, remote, state, metadata,
    created_at, updated_at, last_inbox_read
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name,
    slug = excluded.slug,
    path = excluded.path,
    remote = excluded.remote,
    state = excluded.state,
    metadata = excluded.metadata,
    updated_at = excluded.updated_at,
    last_inbox_read = excluded.last_inbox_read;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;

-- name: FindRecyclableSession :one
SELECT * FROM sessions
WHERE state = 'recycled' AND remote = ?
ORDER BY updated_at ASC
LIMIT 1;

-- name: PublishMessage :exec
INSERT INTO messages (
    id, topic, payload, sender, session_id, created_at
) VALUES (?, ?, ?, ?, ?, ?);

-- name: CountMessagesInTopic :one
SELECT COUNT(*) FROM messages
WHERE topic = ?;

-- name: DeleteOldestMessagesInTopic :exec
DELETE FROM messages
WHERE id IN (
    SELECT id FROM messages AS m
    WHERE m.topic = ?
    ORDER BY m.created_at ASC
    LIMIT ?
);

-- name: SubscribeToTopic :many
SELECT * FROM messages
WHERE topic = ? AND created_at > ?
ORDER BY created_at ASC;

-- name: ListTopics :many
SELECT name FROM topics;

-- name: PruneMessages :exec
DELETE FROM messages
WHERE created_at < ?;

-- name: CountPrunableMessages :one
SELECT COUNT(*) FROM messages
WHERE created_at < ?;
