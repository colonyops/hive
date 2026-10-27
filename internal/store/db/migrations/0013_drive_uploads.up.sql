CREATE TABLE drive_uploads (
    id TEXT PRIMARY KEY,
    server TEXT NOT NULL,
    space TEXT NOT NULL,
    path TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    uploaded_at INTEGER NOT NULL,
    UNIQUE (server, space, path)
);
