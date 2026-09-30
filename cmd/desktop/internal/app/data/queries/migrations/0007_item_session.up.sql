-- The inbox item a queued action ran against (ADR 0060). Empty means the
-- command has no inbox origin — a manual invocation from a surface that has no
-- item behind it.
ALTER TABLE output_command ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE output_command ADD COLUMN source_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE output_command ADD COLUMN source_scope TEXT NOT NULL DEFAULT '';
ALTER TABLE output_command ADD COLUMN external_id TEXT NOT NULL DEFAULT '';

-- Hive sessions this app created on an inbox item's behalf (ADR 0060). Keyed
-- by hive's session id, which is stable across a rename; nothing else about
-- the session is mirrored. The item is named by its durable coordinates rather
-- than inbox_item.id, and so carries no foreign key.
CREATE TABLE item_session (
    session_id   TEXT PRIMARY KEY,
    profile_id   TEXT NOT NULL,
    source_kind  TEXT NOT NULL,
    source_scope TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    created_at   INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_item_session_item
    ON item_session(profile_id, source_kind, source_scope, external_id, created_at DESC);
