-- name: UpsertDocument :one
INSERT INTO documents (id, space, path, title, kind, sha256, size, body, search_text, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (space, path) DO UPDATE SET
    title       = excluded.title,
    kind        = excluded.kind,
    sha256      = excluded.sha256,
    size        = excluded.size,
    body        = excluded.body,
    search_text = excluded.search_text,
    updated_at = excluded.updated_at
RETURNING id, space, path, title, kind, sha256, size, updated_at;

-- name: GetDocument :one
SELECT * FROM documents WHERE space = ? AND path = ?;

-- name: ListDocuments :many
SELECT id, space, path, title, kind, sha256, size, updated_at
FROM documents WHERE space = ? ORDER BY path;

-- name: ListSpaceUsage :many
SELECT space, COUNT(*) AS documents, CAST(SUM(size) AS INTEGER) AS bytes
FROM documents GROUP BY space ORDER BY space;

-- name: DeleteDocument :one
DELETE FROM documents WHERE space = ? AND path = ? RETURNING id;

-- name: HasDocument :one
SELECT EXISTS (SELECT 1 FROM documents WHERE id = ?);

-- name: GetDocumentByID :one
SELECT * FROM documents WHERE id = ?;

-- name: SumDocumentSize :one
SELECT CAST(COALESCE(SUM(size), 0) AS INTEGER) AS bytes FROM documents;

-- name: GetDocumentSize :one
SELECT size FROM documents WHERE space = ? AND path = ?;

-- name: ListUntitledDocuments :many
SELECT id, path, kind, body FROM documents WHERE title = '' LIMIT ?;

-- name: SetDocumentTitle :exec
UPDATE documents SET title = ? WHERE id = ?;

-- name: ListUnindexedHTMLDocuments :many
SELECT id, body FROM documents
WHERE kind = 'html' AND search_text = '' AND id > ?
ORDER BY id LIMIT ?;

-- name: SetDocumentSearchText :exec
UPDATE documents SET search_text = ? WHERE id = ?;
