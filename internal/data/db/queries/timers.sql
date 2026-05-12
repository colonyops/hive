-- name: InsertTimer :exec
INSERT INTO timers (
    id, session_id, tmux_target, prompt, duration_ns,
    fires_at, pid, status, created_at, fired_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateTimerPID :exec
UPDATE timers SET pid = ? WHERE id = ?;

-- name: DeleteTimer :exec
DELETE FROM timers WHERE id = ?;

-- name: GetTimer :one
SELECT * FROM timers WHERE id = ?;

-- name: MarkTimerFired :exec
UPDATE timers SET status = 'fired', fired_at = ? WHERE id = ?;

-- name: MarkTimerFailed :exec
UPDATE timers SET status = 'failed', fired_at = ? WHERE id = ?;

-- name: ActiveTimersForSession :many
SELECT * FROM timers WHERE session_id = ? AND status = 'active'
ORDER BY fires_at ASC;

-- name: ActiveTimersAll :many
SELECT * FROM timers WHERE status = 'active'
ORDER BY fires_at ASC;

-- name: MarkInactiveTimersForSession :exec
UPDATE timers SET status = 'orphaned'
WHERE session_id = ?
  AND status = 'active'
  AND pid IS NOT NULL
  AND id IN (sqlc.slice('ids'));

-- name: MarkInactiveTimersAll :exec
UPDATE timers SET status = 'orphaned'
WHERE status = 'active'
  AND pid IS NOT NULL
  AND id IN (sqlc.slice('ids'));
