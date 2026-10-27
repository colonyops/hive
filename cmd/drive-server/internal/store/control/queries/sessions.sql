-- name: InsertSession :exec
INSERT INTO sessions (id, user_id, value_hash, expires_at, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSessionByValueHash :one
SELECT * FROM sessions WHERE value_hash = ?;

-- name: SetSessionExpiry :exec
UPDATE sessions SET expires_at = ? WHERE id = ?;

-- name: DeleteSessionByValueHash :exec
DELETE FROM sessions WHERE value_hash = ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= ?;
