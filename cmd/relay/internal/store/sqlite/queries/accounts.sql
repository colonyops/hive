-- name: GetAccount :one
SELECT email, encryption, bytes_used, created_at FROM accounts WHERE email = ?;

-- name: PutAccount :exec
INSERT INTO accounts (email, encryption, bytes_used, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(email) DO UPDATE SET encryption = excluded.encryption;

-- name: DeleteAccount :execrows
DELETE FROM accounts WHERE email = ?;

-- name: ListAccounts :many
SELECT email, encryption, bytes_used, created_at FROM accounts ORDER BY email;

-- name: AddAccountBytes :one
UPDATE accounts SET bytes_used = bytes_used + sqlc.arg(delta) WHERE email = sqlc.arg(email)
RETURNING bytes_used;
