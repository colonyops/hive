-- When a chat was pinned to the Code sidebar, in Unix milliseconds; 0 when it
-- is not pinned. Pin order is this column's order.
ALTER TABLE agent_workspace_session ADD COLUMN pinned_at INTEGER NOT NULL DEFAULT 0;
