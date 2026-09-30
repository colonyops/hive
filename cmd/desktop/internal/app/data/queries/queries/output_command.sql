-- name: EnqueueOutputCommand :exec
-- Deduped on (action_id, key): a replayed commit batch enqueues the same
-- action invocation at most once (see idx_output_command_action_key).
INSERT INTO output_command (action_id, key, payload, status, created_at, profile_id, source_kind, source_scope, external_id)
VALUES (?, ?, ?, 'pending', ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- name: ListRunnableOutputCommandsAfter :many
-- Every enqueued flow action is runnable immediately; running is reserved for
-- an explicit detail invocation. Continues a bounded worker scan after the
-- previous row. The status/id predicate is covered by
-- idx_output_command_status_id.
SELECT * FROM output_command
WHERE status = 'pending' AND id > ?
ORDER BY id ASC
LIMIT ?;

-- name: ConfirmOutputCommand :one
-- Explicit detail invocation creates work or claims a queued flow command.
-- Terminal/running commands remain deduplicated. Claiming a queued command
-- keeps the origin the enqueue recorded when that names an item, and takes the
-- caller's only when it does not. The four columns move together: a mix of one
-- row's profile and another's external id would be a reference to no item at
-- all, which is worse than either.
INSERT INTO output_command (action_id, key, payload, status, created_at, profile_id, source_kind, source_scope, external_id)
VALUES (?, ?, ?, 'running', ?, ?, ?, ?, ?)
ON CONFLICT DO UPDATE SET
    status = 'running',
    profile_id = CASE WHEN output_command.profile_id <> '' AND output_command.external_id <> ''
                      THEN output_command.profile_id ELSE excluded.profile_id END,
    source_kind = CASE WHEN output_command.profile_id <> '' AND output_command.external_id <> ''
                       THEN output_command.source_kind ELSE excluded.source_kind END,
    source_scope = CASE WHEN output_command.profile_id <> '' AND output_command.external_id <> ''
                        THEN output_command.source_scope ELSE excluded.source_scope END,
    external_id = CASE WHEN output_command.profile_id <> '' AND output_command.external_id <> ''
                       THEN output_command.external_id ELSE excluded.external_id END
WHERE output_command.status = 'pending'
RETURNING *;

-- name: RerunOutputCommand :one
-- An explicit user confirmation creates a separate command so prior execution
-- diagnostics and Activity links remain intact.
INSERT INTO output_command (action_id, key, payload, status, created_at, is_rerun, profile_id, source_kind, source_scope, external_id)
SELECT sqlc.arg(action_id), sqlc.arg(key), sqlc.arg(payload), 'running', sqlc.arg(created_at), 1,
       sqlc.arg(profile_id), sqlc.arg(source_kind), sqlc.arg(source_scope), sqlc.arg(external_id)
WHERE EXISTS (
    SELECT 1 FROM output_command
    WHERE action_id = sqlc.arg(action_id) AND key = sqlc.arg(key)
      AND status IN ('done', 'failed')
)
AND NOT EXISTS (
    SELECT 1 FROM output_command
    WHERE action_id = sqlc.arg(action_id) AND key = sqlc.arg(key)
      AND status IN ('pending', 'running')
)
RETURNING *;

-- name: GetLatestOutputCommandForAction :one
SELECT * FROM output_command
WHERE action_id = ? AND key = ?
ORDER BY id DESC
LIMIT 1;

-- name: GetOutputCommand :one
SELECT * FROM output_command WHERE id = ?;

-- name: MarkOutputCommandDone :exec
UPDATE output_command
SET status = 'done', last_error = NULL, result_json = ?, stdout = ?, stderr = ?
WHERE id = ?;

-- name: RetryOutputCommand :exec
UPDATE output_command SET attempts = attempts + 1, last_error = ?, stdout = ?, stderr = ?
WHERE id = ?;

-- name: MarkOutputCommandFailed :exec
UPDATE output_command SET status = 'failed', attempts = attempts + 1, last_error = ?, stdout = ?, stderr = ?
WHERE id = ?;

-- name: PruneTerminalOutputCommands :exec
-- Never remove active commands: only terminal done/failed history is bounded.
DELETE FROM output_command
WHERE id IN (
    SELECT id FROM output_command
    WHERE status IN ('done', 'failed')
    ORDER BY id DESC
    LIMIT -1 OFFSET ?
);

-- name: CountNonterminalCommandsForAction :one
-- Pending work can still run and running work may already have been
-- dispatched, so either blocks deleting the action. Done and failed history
-- is terminal and must never keep an action from deletion.
SELECT COUNT(*) FROM output_command
WHERE action_id = ? AND status IN ('pending', 'running');
