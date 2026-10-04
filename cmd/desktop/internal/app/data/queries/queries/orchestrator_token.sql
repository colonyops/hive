-- name: CreateOrchestratorToken :one
INSERT INTO orchestrator_token (name, token_hash, hint, created_at) VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListOrchestratorTokens :many
SELECT * FROM orchestrator_token ORDER BY created_at DESC, id DESC;

-- name: GetOrchestratorTokenByHash :one
SELECT * FROM orchestrator_token WHERE token_hash = ?;

-- name: TouchOrchestratorToken :exec
UPDATE orchestrator_token SET last_used_at = ? WHERE id = ?;

-- name: DeleteOrchestratorToken :execrows
DELETE FROM orchestrator_token WHERE id = ?;
