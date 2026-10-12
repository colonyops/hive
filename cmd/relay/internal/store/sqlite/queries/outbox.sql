-- name: AppendMail :exec
INSERT INTO outbox (id, recipient, subject, body, created_at, next_at, attempts, sent_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: DueMail :many
SELECT id, recipient, subject, body, created_at, next_at, attempts, sent_at FROM outbox
WHERE sent_at IS NULL AND next_at <= sqlc.arg(now)
ORDER BY created_at, id
LIMIT sqlc.arg(max_rows);

-- name: MarkMailSent :execrows
UPDATE outbox SET sent_at = ? WHERE id = ?;

-- name: MarkMailFailed :execrows
UPDATE outbox SET next_at = ?, attempts = attempts + 1 WHERE id = ?;
