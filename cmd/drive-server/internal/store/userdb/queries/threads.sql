-- name: GetDocumentVersion :one
SELECT sha256 FROM documents WHERE id = ?;

-- name: CountThreads :one
SELECT COUNT(*) FROM threads WHERE document_id = ?;

-- name: SumDocumentCommentBytes :one
SELECT CAST(COALESCE(SUM(LENGTH(CAST(c.body AS BLOB))), 0) AS INTEGER) AS bytes
FROM comments c JOIN threads t ON t.id = c.thread_id
WHERE t.document_id = ?;

-- name: InsertThread :exec
INSERT INTO threads (id, document_id, start_line, end_line, quote, anchor_sha256, outdated,
                     author_id, author_email, author_via_token, created_at)
VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?);

-- name: InsertComment :exec
INSERT INTO comments (id, thread_id, author_id, author_email, via_token, body, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetThread :one
SELECT * FROM threads WHERE id = ?;

-- name: ListThreads :many
SELECT * FROM threads WHERE document_id = ?
ORDER BY start_line IS NOT NULL, start_line, created_at, id;

-- name: ListDocumentComments :many
SELECT c.* FROM comments c JOIN threads t ON t.id = c.thread_id
WHERE t.document_id = ?
ORDER BY c.created_at, c.id;

-- name: ListThreadComments :many
SELECT * FROM comments WHERE thread_id = ? ORDER BY created_at, id;

-- name: CountThreadComments :one
SELECT COUNT(*) FROM comments WHERE thread_id = ?;

-- name: CountVisibleThreadComments :one
SELECT COUNT(*) FROM comments WHERE thread_id = ? AND deleted_at IS NULL;

-- name: GetComment :one
SELECT * FROM comments WHERE id = ?;

-- name: UpdateCommentBody :execrows
UPDATE comments SET body = ?, edited_at = ? WHERE id = ? AND deleted_at IS NULL;

-- name: TombstoneComment :exec
UPDATE comments SET body = '', deleted_at = ? WHERE id = ?;

-- name: DeleteComment :exec
DELETE FROM comments WHERE id = ?;

-- name: DeleteThread :exec
DELETE FROM threads WHERE id = ?;

-- name: SetThreadResolved :execrows
UPDATE threads
SET resolved_at = ?, resolved_by = ?, resolved_by_email = ?, resolved_via_token = ?
WHERE id = ?;

-- name: ListAnchoredThreadsBehind :many
SELECT id, start_line, end_line, quote, outdated, anchor_sha256 FROM threads
WHERE document_id = ? AND anchor_sha256 <> ? AND start_line IS NOT NULL;

-- name: UpdateThreadAnchor :execrows
UPDATE threads
SET start_line = ?, end_line = ?, outdated = ?, anchor_sha256 = ?
WHERE id = ? AND document_id = ? AND anchor_sha256 = ?;
