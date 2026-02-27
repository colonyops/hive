CREATE TABLE IF NOT EXISTS hc_items (
    id TEXT PRIMARY KEY,
    repo_key TEXT NOT NULL DEFAULT '',
    epic_id TEXT NOT NULL DEFAULT '',
    parent_id TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    desc TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL DEFAULT 'task',
    status TEXT NOT NULL DEFAULT 'open',
    depth INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_hc_items_repo ON hc_items(repo_key) WHERE repo_key != '';
CREATE INDEX IF NOT EXISTS idx_hc_items_epic ON hc_items(epic_id) WHERE epic_id != '';
CREATE INDEX IF NOT EXISTS idx_hc_items_session ON hc_items(session_id) WHERE session_id != '';
CREATE INDEX IF NOT EXISTS idx_hc_items_status ON hc_items(status);
