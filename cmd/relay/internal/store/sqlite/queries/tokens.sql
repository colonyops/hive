-- name: GetTokenByHash :one
SELECT id, email, hash, kind, device, created_at, last_used_at FROM tokens WHERE hash = ?;

-- name: PutToken :exec
INSERT INTO tokens (id, email, hash, kind, device, created_at, last_used_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  email = excluded.email,
  hash = excluded.hash,
  kind = excluded.kind,
  device = excluded.device,
  created_at = excluded.created_at,
  last_used_at = excluded.last_used_at;

-- name: DeleteToken :execrows
DELETE FROM tokens WHERE email = ? AND id = ?;

-- name: ListTokensByEmail :many
SELECT id, email, hash, kind, device, created_at, last_used_at FROM tokens
WHERE email = ? ORDER BY created_at, id;

-- name: TouchToken :execrows
UPDATE tokens SET last_used_at = ? WHERE id = ?;
