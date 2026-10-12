-- name: GetSource :one
SELECT id, email, name, ingress_hash, secret_hash, created_at FROM sources
WHERE email = ? AND id = ?;

-- name: GetSourceByIngressHash :one
SELECT id, email, name, ingress_hash, secret_hash, created_at FROM sources
WHERE ingress_hash = ?;

-- name: PutSource :exec
INSERT INTO sources (id, email, name, ingress_hash, secret_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  email = excluded.email,
  name = excluded.name,
  ingress_hash = excluded.ingress_hash,
  secret_hash = excluded.secret_hash,
  created_at = excluded.created_at;

-- name: DeleteSource :execrows
DELETE FROM sources WHERE email = ? AND id = ?;

-- name: ListSourcesByEmail :many
SELECT id, email, name, ingress_hash, secret_hash, created_at FROM sources
WHERE email = ? ORDER BY created_at, id;

-- name: CountSourcesByEmail :one
SELECT COUNT(*) FROM sources WHERE email = ?;
