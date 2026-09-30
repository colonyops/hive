-- name: UpsertWebhookCapture :exec
INSERT INTO webhook_capture (topic, received_at, body)
VALUES (?, ?, ?)
ON CONFLICT (topic) DO UPDATE SET received_at = excluded.received_at, body = excluded.body;

-- name: GetWebhookCapture :one
SELECT * FROM webhook_capture WHERE topic = ?;
