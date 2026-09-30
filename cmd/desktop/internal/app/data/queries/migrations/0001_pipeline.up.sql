-- All timestamp columns in this baseline store unix milliseconds.
CREATE TABLE event_log (
    "offset"       INTEGER PRIMARY KEY AUTOINCREMENT,
    topic          TEXT NOT NULL,
    key            TEXT NOT NULL,
    payload        BLOB NOT NULL,
    snapshot       INTEGER NOT NULL DEFAULT 0 CHECK (snapshot IN (0, 1)),
    source_kind    TEXT NOT NULL DEFAULT '',
    source_scope   TEXT NOT NULL DEFAULT '',
    occurrence_key TEXT,
    created_at     INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_event_log_topic_offset ON event_log(topic, "offset");

CREATE TABLE consumer_offset (
    consumer TEXT PRIMARY KEY,
    "offset" INTEGER NOT NULL
) STRICT;

CREATE TABLE source_head (
    topic   TEXT NOT NULL,
    key     TEXT NOT NULL,
    payload BLOB NOT NULL,
    PRIMARY KEY (topic, key)
) STRICT;

CREATE TABLE output_command (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    action_id   TEXT NOT NULL,
    key         TEXT NOT NULL DEFAULT '',
    payload     BLOB NOT NULL,
    status      TEXT NOT NULL,
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT,
    result_json TEXT,
    stdout      TEXT,
    stderr      TEXT,
    created_at  INTEGER NOT NULL
) STRICT;

CREATE UNIQUE INDEX idx_output_command_action_key ON output_command(action_id, key);
CREATE INDEX idx_output_command_status_id ON output_command(status, id);
CREATE INDEX idx_output_command_terminal_id ON output_command(id DESC)
    WHERE status IN ('done', 'failed');

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

CREATE INDEX idx_node_run_ended_at ON node_run(ended_at DESC);
CREATE INDEX idx_node_run_flow_ended_at ON node_run(flow_id, ended_at DESC);

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

CREATE TABLE job (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    status      TEXT NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    step        TEXT NOT NULL DEFAULT '',
    action_id   TEXT NOT NULL DEFAULT '',
    target      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    command_id  INTEGER
) STRICT;

CREATE TABLE inbox_item (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id      TEXT NOT NULL,
    source_kind     TEXT NOT NULL,
    source_scope    TEXT NOT NULL,
    external_id     TEXT NOT NULL,
    title           TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL DEFAULT '',
    payload         BLOB,
    revision        INTEGER NOT NULL DEFAULT 1,
    unread          INTEGER NOT NULL DEFAULT 1 CHECK (unread IN (0, 1)),
    archived_at     INTEGER,
    archived_actor  TEXT CHECK (archived_actor IS NULL OR archived_actor IN ('manual', 'system')),
    archived_reason TEXT,
    lifecycle       TEXT NOT NULL CHECK (lifecycle IN ('active', 'terminal', 'unknown')),
    source_state    TEXT,
    first_seen_at   INTEGER NOT NULL,
    last_event_at   INTEGER NOT NULL,
    UNIQUE (profile_id, source_kind, source_scope, external_id),
    CHECK ((archived_at IS NULL) = (archived_actor IS NULL))
) STRICT;

CREATE INDEX idx_inbox_item_open ON inbox_item(profile_id, last_event_at DESC)
    WHERE archived_at IS NULL;
CREATE INDEX idx_inbox_item_open_unread ON inbox_item(profile_id, last_event_at DESC)
    WHERE archived_at IS NULL AND unread = 1;
CREATE INDEX idx_inbox_item_archive ON inbox_item(profile_id, archived_at DESC)
    WHERE archived_at IS NOT NULL;

CREATE TABLE inbox_event (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id        INTEGER NOT NULL REFERENCES inbox_item(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL,
    transition     TEXT NOT NULL CHECK (transition IN ('none', 'entered-terminal', 'left-terminal')),
    attention      TEXT NOT NULL CHECK (attention IN ('activity', 'trivial')),
    occurrence_key TEXT CHECK (occurrence_key IS NULL OR length(occurrence_key) > 0),
    summary        TEXT,
    detail         BLOB,
    created_at     INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_inbox_event_item ON inbox_event(item_id, id DESC);
CREATE UNIQUE INDEX idx_inbox_event_occurrence ON inbox_event(item_id, occurrence_key)
    WHERE occurrence_key IS NOT NULL;

CREATE TABLE feed_membership_claim (
    profile_id TEXT NOT NULL,
    feed_id    TEXT NOT NULL,
    item_id    INTEGER NOT NULL REFERENCES inbox_item(id) ON DELETE CASCADE,
    source_id  TEXT NOT NULL,
    PRIMARY KEY (profile_id, feed_id, item_id, source_id)
) STRICT;

CREATE INDEX idx_feed_membership_claim_item ON feed_membership_claim(item_id);
