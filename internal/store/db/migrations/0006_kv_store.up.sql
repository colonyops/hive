-- KV store table
CREATE TABLE IF NOT EXISTS kv_store (
    key        TEXT PRIMARY KEY,
    value      BLOB NOT NULL,       -- JSON bytes
    expires_at INTEGER,             -- Unix nanos, NULL = never expires
    created_at INTEGER NOT NULL,    -- Unix nanos
    updated_at INTEGER NOT NULL     -- Unix nanos
);

CREATE INDEX IF NOT EXISTS idx_kv_expires
    ON kv_store(expires_at) WHERE expires_at IS NOT NULL;
