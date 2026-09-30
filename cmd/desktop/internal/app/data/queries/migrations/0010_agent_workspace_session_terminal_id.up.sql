-- A tmux session name lives in a machine-wide namespace, while row ids are
-- only unique inside one database. Preserve existing names and give new chat
-- records an independently generated terminal identity.
ALTER TABLE agent_workspace_session ADD COLUMN terminal_id TEXT NOT NULL DEFAULT '';

UPDATE agent_workspace_session
SET terminal_id = CAST(id AS TEXT);

CREATE UNIQUE INDEX idx_agent_workspace_session_terminal_id
    ON agent_workspace_session(terminal_id);
