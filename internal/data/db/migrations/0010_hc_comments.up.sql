CREATE TABLE IF NOT EXISTS hc_comments (
    id TEXT PRIMARY KEY,
    item_id TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (item_id) REFERENCES hc_items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_hc_comments_item ON hc_comments(item_id);
