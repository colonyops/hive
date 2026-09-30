ALTER TABLE inbox_item ADD COLUMN ignored_at INTEGER;

CREATE INDEX idx_inbox_item_ignored ON inbox_item(profile_id, ignored_at DESC)
    WHERE ignored_at IS NOT NULL;
