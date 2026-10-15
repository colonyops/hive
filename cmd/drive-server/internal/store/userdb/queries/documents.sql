-- name: UpsertDocument :one
INSERT INTO documents (id, space, path, kind, sha256, size, body, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (space, path) DO UPDATE SET
    kind       = excluded.kind,
    sha256     = excluded.sha256,
    size       = excluded.size,
    body       = excluded.body,
    updated_at = excluded.updated_at
RETURNING id, space, path, kind, sha256, size, updated_at;

-- name: GetDocument :one
SELECT * FROM documents WHERE space = ? AND path = ?;

-- name: ListDocuments :many
SELECT id, space, path, kind, sha256, size, updated_at
FROM documents WHERE space = ? ORDER BY path;

-- name: ListSpaceUsage :many
SELECT space, COUNT(*) AS documents, CAST(SUM(size) AS INTEGER) AS bytes
FROM documents GROUP BY space ORDER BY space;

-- name: DeleteDocument :execrows
DELETE FROM documents WHERE space = ? AND path = ?;

-- name: SumDocumentSize :one
SELECT CAST(COALESCE(SUM(size), 0) AS INTEGER) AS bytes FROM documents;

-- name: GetDocumentSize :one
SELECT size FROM documents WHERE space = ? AND path = ?;
