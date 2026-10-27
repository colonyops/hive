-- name: InsertMail :exec
INSERT INTO mail_outbox (id, to_addr, subject, body, next_attempt_at, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListDueMail :many
SELECT * FROM mail_outbox WHERE next_attempt_at <= ? ORDER BY created_at, id LIMIT ?;

-- name: DeleteMail :exec
DELETE FROM mail_outbox WHERE id = ?;

-- name: DeferMail :exec
UPDATE mail_outbox SET attempts = attempts + 1, next_attempt_at = ? WHERE id = ?;
