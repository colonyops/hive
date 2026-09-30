-- A session created from a multi-selection belongs to every selected inbox
-- item. Preserve the existing one-item rows while widening the key.
ALTER TABLE item_session RENAME TO item_session_old;

CREATE TABLE item_session (
    session_id   TEXT NOT NULL,
    profile_id   TEXT NOT NULL,
    source_kind  TEXT NOT NULL,
    source_scope TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    PRIMARY KEY (session_id, profile_id, source_kind, source_scope, external_id)
) STRICT;

INSERT INTO item_session (session_id, profile_id, source_kind, source_scope, external_id, created_at)
SELECT session_id, profile_id, source_kind, source_scope, external_id, created_at
FROM item_session_old;

DROP TABLE item_session_old;

CREATE INDEX idx_item_session_item
    ON item_session(profile_id, source_kind, source_scope, external_id, created_at DESC);
