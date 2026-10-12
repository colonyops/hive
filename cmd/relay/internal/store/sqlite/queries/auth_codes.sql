-- name: PutAuthCode :exec
INSERT INTO auth_codes (email, hash, expires_at, attempts)
VALUES (?, ?, ?, 0)
ON CONFLICT(email) DO UPDATE SET
  hash = excluded.hash,
  expires_at = excluded.expires_at,
  attempts = 0;

-- name: GetAuthCode :one
SELECT email, hash, expires_at, attempts FROM auth_codes WHERE email = ?;

-- name: DeleteAuthCode :exec
DELETE FROM auth_codes WHERE email = ?;

-- name: IncrementAuthCodeAttempts :one
UPDATE auth_codes SET attempts = attempts + 1 WHERE email = ?
RETURNING attempts;

-- name: DeleteExpiredAuthCodes :execrows
DELETE FROM auth_codes WHERE expires_at <= ?;
