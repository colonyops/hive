-- name: InsertAccount :exec
INSERT INTO accounts (id, email, created_at)
VALUES (?, ?, ?)
ON CONFLICT (email) DO NOTHING;

-- name: GetAccountByEmail :one
SELECT * FROM accounts WHERE email = ?;

-- name: GetAccountByID :one
SELECT * FROM accounts WHERE id = ?;

-- name: ListAccounts :many
SELECT * FROM accounts ORDER BY created_at, id;

-- name: DeleteAccountByEmail :execrows
DELETE FROM accounts WHERE email = ?;

-- name: DeleteSignInCodesByEmail :exec
DELETE FROM sign_in_codes WHERE email = ?;
