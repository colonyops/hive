-- event_log is the append-only pipeline event log. "offset" is a SQLite
-- keyword when unquoted as a column name, so it is quoted consistently here
-- and in every query that references it.
CREATE TABLE event_log (
    "offset"   INTEGER PRIMARY KEY AUTOINCREMENT,
    topic      TEXT NOT NULL,
    key        TEXT NOT NULL,
    payload    BLOB NOT NULL,
    created_at INTEGER NOT NULL -- unix nanoseconds
) STRICT;

CREATE INDEX idx_event_log_topic_offset ON event_log(topic, "offset");

-- consumer_offset tracks the last committed read offset per consumer.
CREATE TABLE consumer_offset (
    consumer TEXT PRIMARY KEY,
    "offset" INTEGER NOT NULL
) STRICT;

-- feed_item is the persisted, Go-owned output of a flow's feed nodes.
CREATE TABLE feed_item (
    feed_id    TEXT NOT NULL,
    item_id    TEXT NOT NULL,
    payload    BLOB NOT NULL,
    updated_at INTEGER NOT NULL,
    unread     INTEGER NOT NULL,
    PRIMARY KEY (feed_id, item_id)
) STRICT;

-- output_command is the queue of side-effecting outputs (actions) waiting
-- for the output worker to execute them.
CREATE TABLE output_command (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    action_id  TEXT NOT NULL,
    payload    BLOB NOT NULL,
    status     TEXT NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

-- node_run records per-node execution metrics for the flows debug/status UI.
CREATE TABLE node_run (
    flow_id    TEXT NOT NULL,
    node_id    TEXT NOT NULL,
    ok         INTEGER NOT NULL,
    in_count   INTEGER NOT NULL,
    out_count  INTEGER NOT NULL,
    drop_count INTEGER NOT NULL,
    err        TEXT,
    ended_at   INTEGER NOT NULL,
    dur_ms     INTEGER NOT NULL
) STRICT;
