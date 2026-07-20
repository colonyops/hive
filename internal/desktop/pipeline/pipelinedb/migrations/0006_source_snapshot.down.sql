DROP INDEX IF EXISTS idx_feed_item_source_snapshot;
ALTER TABLE feed_item DROP COLUMN snapshot_id;
ALTER TABLE feed_item DROP COLUMN source_topic;
ALTER TABLE event_log DROP COLUMN snapshot;
