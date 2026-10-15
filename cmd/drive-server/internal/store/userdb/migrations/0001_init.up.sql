CREATE TABLE documents (
    id         TEXT PRIMARY KEY,
    space      TEXT NOT NULL,
    path       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    sha256     TEXT NOT NULL,
    size       INTEGER NOT NULL,
    body       BLOB NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (space, path)
);
