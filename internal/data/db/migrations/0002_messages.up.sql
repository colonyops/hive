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

-- Topics view (distinct topics with last update time)
CREATE VIEW IF NOT EXISTS topics AS
SELECT
    topic AS name,
    MAX(created_at) AS updated_at
FROM messages
GROUP BY topic
ORDER BY updated_at DESC;
