-- One named conversation with an agent in one workspace. The record persists;
-- the process does not — every ptyterm terminal dies with the app (ADR 0048),
-- so reopening relaunches the agent with its own resume flag. Hive stores no
-- transcript: the agent owns its history.
--
-- workspace is the directory name under the root, not a path, so a record
-- survives a machine whose root is configured somewhere else. Two sessions may
-- share a name; the row id is the identity.
CREATE TABLE agent_workspace_session (
    id               INTEGER PRIMARY KEY,
    workspace        TEXT NOT NULL,
    name             TEXT NOT NULL,
    agent            TEXT NOT NULL,
    agent_session_id TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    last_opened_at   INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_agent_workspace_session_workspace
    ON agent_workspace_session(workspace, last_opened_at DESC);
