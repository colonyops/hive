-- name: EnqueueOutputCommand :exec
-- Deduped on (action_id, key): a replayed commit batch enqueues the same
-- action invocation at most once (see idx_output_command_action_key).
INSERT INTO output_command (action_id, key, payload, status, created_at, profile_id, source_kind, source_scope, external_id)
VALUES (?, ?, ?, 'pending', ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- name: ListRunnableOutputCommandsAfter :many
SELECT * FROM output_command
WHERE status = 'pending' AND dispatch_lane = 'automatic' AND id > ?
ORDER BY id ASC
LIMIT ?;

-- name: ClaimNextAutomaticOutputCommand :one
UPDATE output_command
SET status = 'running', claim_token = sqlc.arg(claim_token), claimed_at = sqlc.arg(claimed_at),
    attempts = attempts + 1, finished_at = 0
WHERE id = (
    SELECT oc.id FROM output_command oc
    WHERE oc.status = 'pending' AND oc.dispatch_lane = 'automatic'
      AND oc.id > sqlc.arg(after_id) AND oc.not_before <= sqlc.arg(claimed_at)
    ORDER BY oc.id ASC
    LIMIT 1
)
AND status = 'pending'
RETURNING *;

-- name: ConfirmOutputCommand :one
INSERT INTO output_command (
    action_id, key, payload, status, attempts, created_at, dispatch_lane,
    claim_token, claimed_at, profile_id, source_kind, source_scope, external_id
)
VALUES (?, ?, ?, 'running', 1, ?, 'manual', ?, ?, ?, ?, ?, ?)
ON CONFLICT DO UPDATE SET
    status = 'running',
    attempts = output_command.attempts + 1,
    dispatch_lane = 'manual',
    claim_token = excluded.claim_token,
    claimed_at = excluded.claimed_at,
    finished_at = 0,
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
INSERT INTO output_command (
    action_id, key, payload, status, attempts, created_at, is_rerun, dispatch_lane,
    claim_token, claimed_at, profile_id, source_kind, source_scope, external_id
)
SELECT sqlc.arg(action_id), sqlc.arg(key), sqlc.arg(payload), 'running', 1, sqlc.arg(created_at), 1, 'manual',
       sqlc.arg(claim_token), sqlc.arg(claimed_at), sqlc.arg(profile_id), sqlc.arg(source_kind),
       sqlc.arg(source_scope), sqlc.arg(external_id)
WHERE EXISTS (
    SELECT 1 FROM output_command
    WHERE action_id = sqlc.arg(action_id) AND key = sqlc.arg(key)
      AND status IN ('done', 'failed', 'cancelled')
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

-- name: CompleteClaimedOutputCommand :execrows
UPDATE output_command
SET status = 'done', claim_token = '', finished_at = ?, last_error = NULL, result_json = ?, stdout = ?, stderr = ?
WHERE id = ? AND status = 'running' AND claim_token = ?;

-- name: RequeueClaimedOutputCommand :execrows
UPDATE output_command
SET status = 'pending', claim_token = '', claimed_at = 0, not_before = ?,
    last_error = ?, stdout = ?, stderr = ?
WHERE id = ? AND status = 'running' AND claim_token = ?;

-- name: FailClaimedOutputCommand :execrows
UPDATE output_command
SET status = 'failed', claim_token = '', finished_at = ?, last_error = ?, stdout = ?, stderr = ?
WHERE id = ? AND status = 'running' AND claim_token = ?;

-- name: CancelClaimedOutputCommand :execrows
UPDATE output_command
SET status = 'cancelled', claim_token = '', finished_at = ?, last_error = ?, stdout = ?, stderr = ?
WHERE id = ? AND status = 'running' AND claim_token = ?;

-- name: PruneTerminalOutputCommands :exec
-- Never remove active commands: only terminal history is bounded.
DELETE FROM output_command
WHERE id IN (
    SELECT oc.id FROM output_command oc
    WHERE oc.status IN ('done', 'failed', 'cancelled')
      AND NOT (
          oc.action_id LIKE 'launch:%'
          AND EXISTS (
              SELECT 1 FROM inbox_item i
              WHERE i.profile_id = oc.profile_id
                AND i.source_kind = oc.source_kind
                AND i.external_id = oc.external_id
          )
      )
    ORDER BY oc.id DESC
    LIMIT -1 OFFSET ?
);

-- name: CountNonterminalCommandsForAction :one
SELECT COUNT(*) FROM output_command
WHERE action_id = ? AND status IN ('pending', 'running');
