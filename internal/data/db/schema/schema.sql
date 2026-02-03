-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);

-- Initialize schema version
INSERT OR IGNORE INTO schema_version (version) VALUES (2);

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    path TEXT NOT NULL,
    remote TEXT NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('active', 'recycled', 'corrupted')),
    metadata TEXT, -- JSON blob for map[string]string
    created_at INTEGER NOT NULL, -- Unix timestamp in nanoseconds
    updated_at INTEGER NOT NULL -- Unix timestamp in nanoseconds
);

-- Index for FindRecyclable query (finds recycled sessions with specific remote)
CREATE INDEX IF NOT EXISTS idx_sessions_state_remote ON sessions(state, remote);

-- Messages table
CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    topic TEXT NOT NULL,
    payload TEXT NOT NULL,
    sender TEXT,
    session_id TEXT,
    created_at INTEGER NOT NULL -- Unix timestamp in nanoseconds
);

-- Index for Subscribe queries (filter by topic, order by created_at)
CREATE INDEX IF NOT EXISTS idx_messages_topic_created ON messages(topic, created_at);

-- Message acknowledgment tracking
CREATE TABLE IF NOT EXISTS message_reads (
    message_id TEXT NOT NULL,
    consumer_id TEXT NOT NULL,  -- Session ID directly (no prefix)
    read_at INTEGER NOT NULL,   -- Unix nanoseconds
    PRIMARY KEY (message_id, consumer_id)
);

CREATE INDEX IF NOT EXISTS idx_message_reads_consumer ON message_reads(consumer_id, read_at);
CREATE INDEX IF NOT EXISTS idx_message_reads_message ON message_reads(message_id);

-- Topics view (distinct topics with last update time)
CREATE VIEW IF NOT EXISTS topics AS
SELECT
    topic AS name,
    MAX(created_at) AS updated_at
FROM messages
GROUP BY topic
ORDER BY updated_at DESC;
