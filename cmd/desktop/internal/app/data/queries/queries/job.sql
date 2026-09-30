-- name: InsertJob :one
INSERT INTO job (created_at, updated_at, status, label, step, action_id, target, error, command_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: SetJobRunning :one
-- Advance a job to running and link its output_command. This is the ONLY write
-- that sets command_id, so terminal transitions can never null it out.
UPDATE job SET updated_at = ?, status = ?, step = ?, command_id = ?
WHERE id = ?
RETURNING *;

-- name: SetJobStatus :one
-- Advance a job's status/step/error WITHOUT touching command_id (used by the
-- done/failed terminal transitions). Splitting this from SetJobRunning avoids a
-- read-modify-write and prevents clobbering the command link.
UPDATE job SET updated_at = ?, status = ?, step = ?, error = ?
WHERE id = ?
RETURNING *;

-- name: FindRunningJobByCommandID :one
-- Restore a running job's identity after a worker or app restart so retries do
-- not create duplicate jobs and strand the original lifecycle as active.
SELECT * FROM job
WHERE command_id = ? AND status = 'running'
ORDER BY id DESC
LIMIT 1;

-- name: ListJobs :many
-- Newest first, paged by a descending id cursor (same shape as ListActivityEvents).
SELECT * FROM job WHERE id < ? ORDER BY id DESC LIMIT ?;

-- name: ListActiveJobs :many
-- Non-terminal jobs plus terminal jobs updated within a recency window, newest
-- first. Drives the auto-hiding titlebar chip.
SELECT * FROM job
WHERE status IN ('queued', 'running') OR (status IN ('done', 'failed') AND updated_at >= ?)
ORDER BY id DESC;

-- name: PruneTerminalJobs :exec
-- Never remove active jobs: only terminal done/failed history is bounded
-- (mirrors PruneTerminalOutputCommands).
DELETE FROM job
WHERE id IN (
    SELECT id FROM job
    WHERE status IN ('done', 'failed')
    ORDER BY id DESC
    LIMIT -1 OFFSET ?
);
