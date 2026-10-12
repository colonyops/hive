-- name: PutRecipient :exec
INSERT INTO recipients (id, email, recipient, device, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(email, recipient) DO UPDATE SET device = excluded.device;

-- name: DeleteRecipient :execrows
DELETE FROM recipients WHERE email = ? AND id = ?;

-- name: ListRecipientsByEmail :many
SELECT id, email, recipient, device, created_at FROM recipients
WHERE email = ? ORDER BY created_at, id;
