-- activity_event is the user-facing audit log surfaced by the desktop's
-- Activity view: refreshes, sessions created, automatic and manual actions,
-- config reloads, and errors. Unlike event_log (the pipeline data plane that
-- the graph runtime consumes), these rows exist for a human to debug, inform,
-- or track what the app did. Any backend subsystem or the frontend appends one
-- through the activity.Recorder, so the schema is deliberately generic.
--
-- category/severity are stored as plain TEXT: the activity package owns the
-- typed enums and converts at its boundary, keeping pipelinedb a leaf store
-- (the same split as feed_item.unread being a bool only above the row layer).
-- created_at is unix MILLISECONDS (not the nanoseconds used by event_log) so it
-- survives the int64->JS number boundary in the Wails bindings that render it.
-- metadata is optional raw JSON for later enrichment (links, counts, ids)
-- without a schema change.
CREATE TABLE activity_event (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at INTEGER NOT NULL,
    category   TEXT NOT NULL,
    severity   TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    source     TEXT NOT NULL DEFAULT '',
    metadata   BLOB
) STRICT;
