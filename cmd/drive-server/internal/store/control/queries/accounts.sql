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

-- name: SetAccountLastSignIn :exec
UPDATE accounts SET last_sign_in_at = ? WHERE id = ?;
