CREATE TABLE IF NOT EXISTS hc_activity (
    id TEXT PRIMARY KEY,
    item_id TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'update',
    message TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (item_id) REFERENCES hc_items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_hc_activity_item ON hc_activity(item_id);
CREATE INDEX IF NOT EXISTS idx_hc_activity_type ON hc_activity(type);
