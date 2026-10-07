-- name: InsertActionRunLogLine :exec
INSERT INTO action_run_log (command_id, attempt, stream, text, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: ListActionRunLogAfter :many
SELECT * FROM action_run_log
WHERE command_id = ? AND id > ?
ORDER BY id ASC
LIMIT ?;

-- name: ActionRunLogIDFromEnd :one
-- The id of the line `offset` lines before the newest, for a tail read.
SELECT id FROM action_run_log
WHERE command_id = ?
ORDER BY id DESC
LIMIT 1 OFFSET ?;

-- name: ListActionRuns :many
-- Only actions.yml actions: an authored id is a slug, and the synthetic ids of
-- notify and launch nodes are the only ones that contain a colon.
SELECT
    oc.id, oc.action_id, oc.key, oc.status, oc.attempts, oc.dispatch_lane, oc.is_rerun,
    oc.created_at, oc.claimed_at, oc.finished_at, oc.last_error,
    oc.profile_id, oc.source_kind, oc.source_scope, oc.external_id,
    CAST(COALESCE((SELECT j.label FROM job j WHERE j.command_id = oc.id ORDER BY j.id DESC LIMIT 1), '') AS TEXT) AS label,
    CAST(COALESCE(i.id, 0) AS INTEGER) AS item_id,
    CAST(COALESCE(i.title, '') AS TEXT) AS item_title
FROM output_command oc
LEFT JOIN inbox_item i
    ON oc.external_id <> ''
   AND i.profile_id = oc.profile_id AND i.source_kind = oc.source_kind
   AND i.source_scope = oc.source_scope AND i.external_id = oc.external_id
WHERE oc.id < sqlc.arg(before_id) AND instr(oc.action_id, ':') = 0
ORDER BY oc.id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetActionRun :one
SELECT
    oc.id, oc.action_id, oc.key, oc.status, oc.attempts, oc.dispatch_lane, oc.is_rerun,
    oc.created_at, oc.claimed_at, oc.finished_at, oc.last_error,
    oc.profile_id, oc.source_kind, oc.source_scope, oc.external_id,
    CAST(COALESCE((SELECT j.label FROM job j WHERE j.command_id = oc.id ORDER BY j.id DESC LIMIT 1), '') AS TEXT) AS label,
    CAST(COALESCE(i.id, 0) AS INTEGER) AS item_id,
    CAST(COALESCE(i.title, '') AS TEXT) AS item_title
FROM output_command oc
LEFT JOIN inbox_item i
    ON oc.external_id <> ''
   AND i.profile_id = oc.profile_id AND i.source_kind = oc.source_kind
   AND i.source_scope = oc.source_scope AND i.external_id = oc.external_id
WHERE oc.id = ?;
