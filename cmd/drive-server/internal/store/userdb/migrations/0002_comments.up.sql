-- A thread with no start_line is a general thread. anchor_sha256 is the
-- document version the anchor was last checked against: a push re-anchors
-- exactly the threads still on the version it replaced.
CREATE TABLE threads (
    id                 TEXT PRIMARY KEY,
    document_id        TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    start_line         INTEGER,
    end_line           INTEGER,
    quote              TEXT,
    anchor_sha256      TEXT NOT NULL,
    outdated           INTEGER NOT NULL DEFAULT 0,
    author_id          TEXT NOT NULL,
    author_email       TEXT NOT NULL,
    author_via_token   TEXT NOT NULL DEFAULT '',
    resolved_at        INTEGER,
    resolved_by        TEXT,
    resolved_by_email  TEXT,
    resolved_via_token TEXT NOT NULL DEFAULT '',
    created_at         INTEGER NOT NULL
);
CREATE INDEX threads_document ON threads (document_id, start_line);

-- author_email is kept so a comment still reads after its account is gone.
CREATE TABLE comments (
    id           TEXT PRIMARY KEY,
    thread_id    TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    author_id    TEXT NOT NULL,
    author_email TEXT NOT NULL,
    via_token    TEXT NOT NULL DEFAULT '',
    body         TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    edited_at    INTEGER,
    deleted_at   INTEGER
);
CREATE INDEX comments_thread ON comments (thread_id, created_at, id);
