-- name: ListAgentWorkspaceSessions :many
-- One workspace's sessions, newest record first. Creation order on purpose,
-- not last_opened_at: resuming a chat must not reshuffle the sidebar under
-- the pointer.
SELECT * FROM agent_workspace_session
WHERE workspace = ?
ORDER BY id DESC;

-- name: GetAgentWorkspaceSession :one
SELECT * FROM agent_workspace_session WHERE id = ?;

-- name: InsertAgentWorkspaceSession :one
INSERT INTO agent_workspace_session (workspace, name, agent, agent_session_id, terminal_id, created_at, last_opened_at, schedule_id, end_token)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAgentWorkspaceSessionByEndToken :one
-- The session a launch handed this token to. An empty token matches nothing:
-- a row from before the column existed must not answer for a blank bearer.
SELECT * FROM agent_workspace_session WHERE end_token = ? AND end_token <> '';

-- name: TouchAgentWorkspaceSession :exec
UPDATE agent_workspace_session SET last_opened_at = ? WHERE id = ?;

-- name: SetAgentWorkspaceSessionAgentID :exec
-- The only write to agent_session_id after the insert: a resume that falls
-- back to a fresh launch (the agent has no resume form) mints a new id and
-- records it here so a later resume of this same record addresses the
-- conversation actually running rather than the one it replaced.
UPDATE agent_workspace_session SET agent_session_id = ? WHERE id = ?;

-- name: RenameAgentWorkspaceSession :exec
UPDATE agent_workspace_session SET name = ? WHERE id = ?;

-- name: DeleteAgentWorkspaceSession :exec
DELETE FROM agent_workspace_session WHERE id = ?;

-- name: DeleteAgentWorkspaceSessionsByWorkspace :exec
DELETE FROM agent_workspace_session WHERE workspace = ?;

-- name: ListAllAgentWorkspaceSessions :many
-- Every session across every workspace, newest record first: the same
-- stable creation order the scoped list uses.
SELECT * FROM agent_workspace_session
ORDER BY id DESC;
