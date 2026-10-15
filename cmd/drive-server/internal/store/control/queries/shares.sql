-- name: InsertShareLinkIfAbsent :exec
INSERT INTO share_links (id, slug, user_id, document_id, expires_at, expires_in, created_at)
SELECT sqlc.arg(id), sqlc.arg(slug), sqlc.arg(user_id), sqlc.arg(document_id), sqlc.arg(expires_at), sqlc.arg(expires_in), sqlc.arg(created_at)
WHERE NOT EXISTS (
    SELECT 1 FROM share_links WHERE user_id = sqlc.arg(user_id) AND document_id = sqlc.arg(document_id)
);

-- name: GetShareLinkBySlug :one
SELECT * FROM share_links WHERE slug = ?;

-- name: GetShareLinkByDocument :one
SELECT * FROM share_links WHERE user_id = ? AND document_id = ?;

-- name: ListShareLinksByUser :many
SELECT * FROM share_links WHERE user_id = ? ORDER BY id;

-- name: ListShareLinksAfter :many
SELECT * FROM share_links WHERE id > ? ORDER BY id LIMIT ?;

-- name: UpdateShareLinkExpiry :exec
UPDATE share_links SET expires_at = ?, expires_in = ? WHERE id = ?;

-- name: DeleteShareLink :exec
DELETE FROM share_links WHERE id = ?;

-- name: DeleteShareLinksByDocument :exec
DELETE FROM share_links WHERE user_id = ? AND document_id = ?;

-- name: DeleteExpiredShareLinks :execrows
DELETE FROM share_links WHERE expires_at IS NOT NULL AND expires_at <= ?;
