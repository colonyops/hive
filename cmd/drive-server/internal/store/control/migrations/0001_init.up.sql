CREATE TABLE accounts (
    id              TEXT PRIMARY KEY,
    email           TEXT NOT NULL UNIQUE,
    created_at      INTEGER NOT NULL,
    last_sign_in_at INTEGER
);

CREATE TABLE sign_in_codes (
    id         TEXT PRIMARY KEY,
    email      TEXT NOT NULL,
    code_hash  TEXT NOT NULL UNIQUE,
    attempts   INTEGER NOT NULL DEFAULT 0,
    expires_at INTEGER NOT NULL
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    value_hash TEXT NOT NULL UNIQUE,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE api_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    secret_hash  TEXT NOT NULL UNIQUE,
    scope        TEXT NOT NULL,
    expires_at   INTEGER,
    last_used_at INTEGER,
    created_at   INTEGER NOT NULL
);

CREATE TABLE share_links (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    user_id     TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    document_id TEXT NOT NULL,
    expires_at  INTEGER,
    expires_in  TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE INDEX share_links_document ON share_links (user_id, document_id);

CREATE TABLE mail_outbox (
    id              TEXT PRIMARY KEY,
    to_addr         TEXT NOT NULL,
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL,
    created_at      INTEGER NOT NULL
);
