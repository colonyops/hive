-- name: ListSessions :many
SELECT * FROM sessions
ORDER BY created_at DESC;

-- name: GetSession :one
SELECT * FROM sessions
WHERE id = ?;

-- name: SaveSession :exec
INSERT INTO sessions (
    id, name, slug, path, remote, state, metadata,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name,
    slug = excluded.slug,
    path = excluded.path,
    remote = excluded.remote,
    state = excluded.state,
    metadata = excluded.metadata,
    updated_at = excluded.updated_at;

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
SELECT name FROM topics
ORDER BY name ASC;

-- name: PruneMessages :exec
DELETE FROM messages
WHERE created_at < ?;

-- name: CountPrunableMessages :one
SELECT COUNT(*) FROM messages
WHERE created_at < ?;

-- name: AcknowledgeMessages :exec
INSERT INTO message_reads (message_id, consumer_id, read_at)
VALUES (?, ?, ?)
ON CONFLICT (message_id, consumer_id) DO UPDATE SET
    read_at = excluded.read_at;

-- name: GetUnreadMessages :many
SELECT m.id, m.topic, m.payload, m.sender, m.session_id, m.created_at
FROM messages m
LEFT JOIN message_reads mr ON mr.message_id = m.id AND mr.consumer_id = ?
WHERE m.topic = ?
  AND mr.message_id IS NULL
ORDER BY m.created_at ASC;

-- name: CreateReviewSession :exec
INSERT INTO review_sessions (
    id, document_path, content_hash, created_at, finalized_at
) VALUES (?, ?, ?, ?, ?);

-- name: GetReviewSessionByDocPath :one
SELECT * FROM review_sessions
WHERE document_path = ?
ORDER BY created_at DESC
LIMIT 1;

-- name: GetReviewSessionByDocPathAndHash :one
SELECT * FROM review_sessions
WHERE document_path = ? AND content_hash = ?;

-- name: FinalizeReviewSession :exec
UPDATE review_sessions
SET finalized_at = ?
WHERE id = ?;

-- name: DeleteReviewSession :exec
DELETE FROM review_sessions
WHERE id = ?;

-- name: DeleteReviewSessionsByDocPath :exec
DELETE FROM review_sessions
WHERE document_path = ? AND content_hash != ?;

-- name: SaveReviewComment :exec
INSERT INTO review_comments (
    id, session_id, start_line, end_line, context_text, comment_text, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListReviewComments :many
SELECT * FROM review_comments
WHERE session_id = ?
ORDER BY start_line ASC;

-- name: DeleteReviewComment :exec
DELETE FROM review_comments
WHERE id = ?;
