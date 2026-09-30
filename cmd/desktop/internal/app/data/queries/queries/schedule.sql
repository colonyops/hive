-- name: GetScheduleCursor :one
SELECT * FROM schedule_cursor WHERE workspace = ? AND schedule_id = ?;

-- name: UpsertScheduleCursor :exec
INSERT INTO schedule_cursor (workspace, schedule_id, evaluated_through, cron)
VALUES (?, ?, ?, ?)
ON CONFLICT (workspace, schedule_id) DO UPDATE SET
    evaluated_through = excluded.evaluated_through, cron = excluded.cron;

-- name: ListScheduleCursors :many
SELECT * FROM schedule_cursor;

-- name: DeleteScheduleCursor :exec
DELETE FROM schedule_cursor WHERE workspace = ? AND schedule_id = ?;

-- name: DeleteScheduleCursorsByWorkspace :exec
DELETE FROM schedule_cursor WHERE workspace = ?;

-- name: InsertScheduleRun :one
INSERT INTO schedule_run (
    workspace, schedule_id, schedule_name, scheduled_for, started_at,
    reason, missed, status, session_id, prompt, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListScheduleRunsForSchedule :many
-- One schedule's runs, newest first.
SELECT * FROM schedule_run
WHERE workspace = ? AND schedule_id = ?
ORDER BY started_at DESC, id DESC
LIMIT ?;

-- name: LastLaunchedScheduleRun :one
-- The most recent run that actually started a chat, skipping over failed and
-- skipped attempts. The scheduler checks this before deciding whether the
-- previous run's chat is still live.
SELECT * FROM schedule_run
WHERE workspace = ? AND schedule_id = ? AND status = 'launched'
ORDER BY started_at DESC, id DESC
LIMIT 1;

-- name: PruneScheduleRuns :exec
-- Retain only the newest `keep` rows for one (workspace, schedule_id), the
-- per-schedule counterpart to PruneNodeRuns' global bound. id breaks ties on
-- equal started_at.
DELETE FROM schedule_run AS target
WHERE target.workspace = sqlc.arg(workspace) AND target.schedule_id = sqlc.arg(schedule_id)
  AND target.id NOT IN (
      SELECT kept.id FROM schedule_run AS kept
      WHERE kept.workspace = sqlc.arg(workspace) AND kept.schedule_id = sqlc.arg(schedule_id)
      ORDER BY kept.started_at DESC, kept.id DESC
      LIMIT sqlc.arg(keep)
  );

-- name: DeleteScheduleRunsByWorkspace :exec
DELETE FROM schedule_run WHERE workspace = ?;
