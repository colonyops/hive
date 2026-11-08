-- A project session: one agent root over one hive session per repository.
CREATE TABLE project_sessions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    project TEXT NOT NULL,
    root TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- A child session belongs to at most one project session. Deleting the child
-- row drops it from the project; the project service refuses that path, but
-- a recycled child replaced by a new session must not leave a dangling link.
CREATE TABLE project_session_repos (
    project_session_id TEXT NOT NULL REFERENCES project_sessions(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL UNIQUE REFERENCES sessions(id) ON DELETE CASCADE,
    dir TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (project_session_id, dir)
);
