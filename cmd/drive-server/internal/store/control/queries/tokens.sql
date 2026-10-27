-- name: InsertAPIToken :exec
INSERT INTO api_tokens (id, user_id, name, secret_hash, scope, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetAPITokenBySecretHash :one
SELECT * FROM api_tokens WHERE secret_hash = ?;

-- name: ListAPITokensByUser :many
SELECT * FROM api_tokens WHERE user_id = ? ORDER BY created_at, id;

-- name: DeleteAPIToken :execrows
DELETE FROM api_tokens WHERE id = ? AND user_id = ?;

-- name: TouchAPIToken :exec
UPDATE api_tokens SET last_used_at = ? WHERE id = ?;
