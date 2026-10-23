-- name: DeleteSignInCodesForEmail :exec
DELETE FROM sign_in_codes WHERE email = ?;

-- name: InsertSignInCode :exec
INSERT INTO sign_in_codes (id, email, code_hash, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetSignInCodeByEmail :one
SELECT * FROM sign_in_codes WHERE email = ?;

-- name: BumpSignInCodeAttempts :exec
UPDATE sign_in_codes SET attempts = attempts + 1 WHERE id = ?;

-- name: DeleteSignInCode :exec
DELETE FROM sign_in_codes WHERE id = ?;

-- name: DeleteExpiredSignInCodes :execrows
DELETE FROM sign_in_codes WHERE expires_at <= ?;
