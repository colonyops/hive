-- Project sessions

-- name: ListProjectSessions :many
SELECT * FROM project_sessions ORDER BY created_at DESC;

-- name: GetProjectSession :one
SELECT * FROM project_sessions WHERE id = ?;

-- name: UpsertProjectSession :exec
INSERT INTO project_sessions (id, name, slug, project, root, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    name = excluded.name,
    slug = excluded.slug,
    project = excluded.project,
    root = excluded.root,
    updated_at = excluded.updated_at;

-- name: DeleteProjectSession :execrows
DELETE FROM project_sessions WHERE id = ?;

-- name: ListProjectSessionRepos :many
SELECT * FROM project_session_repos WHERE project_session_id = ? ORDER BY position;

-- name: DeleteProjectSessionRepos :exec
DELETE FROM project_session_repos WHERE project_session_id = ?;

-- name: InsertProjectSessionRepo :exec
INSERT INTO project_session_repos (project_session_id, session_id, dir, position)
VALUES (?, ?, ?, ?);

-- name: ProjectSessionIDForSession :one
SELECT project_session_id FROM project_session_repos WHERE session_id = ?;
