-- Snapshot events carry a source's complete current item set. They let flow
-- consumers reconcile persisted feed outputs when an upstream item disappears
-- or stops matching a downstream filter.
ALTER TABLE event_log ADD COLUMN snapshot INTEGER NOT NULL DEFAULT 0;

-- Feed rows retain the source snapshot that most recently produced them.
-- Reconciliation removes rows for one source/feed scope that were not seen in
-- its latest successful snapshot, without affecting other sources in a shared
-- feed.
ALTER TABLE feed_item ADD COLUMN source_topic TEXT NOT NULL DEFAULT '';
ALTER TABLE feed_item ADD COLUMN snapshot_id TEXT NOT NULL DEFAULT '';

-- Existing rows have no recoverable source or snapshot provenance. Leaving them
-- with the empty defaults would prevent a real source snapshot from ever
-- reconciling them, so this intentionally breaking migration clears them.
DELETE FROM feed_item;

CREATE INDEX idx_feed_item_source_snapshot ON feed_item(feed_id, source_topic, snapshot_id);
