-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);

-- Initialize schema version
INSERT OR IGNORE INTO schema_version (version) VALUES (5);

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

-- Review sessions table
CREATE TABLE IF NOT EXISTS review_sessions (
    id TEXT PRIMARY KEY,
    document_path TEXT NOT NULL,          -- Absolute path to document
    content_hash TEXT NOT NULL,           -- SHA256 hash of document content
    created_at INTEGER NOT NULL,          -- Unix timestamp in nanoseconds
    finalized_at INTEGER,                 -- Unix timestamp in nanoseconds, NULL if not finalized
    session_name TEXT DEFAULT '' NOT NULL, -- Human-readable name for diff sessions
    diff_context TEXT DEFAULT '' NOT NULL, -- Git context for diff sessions (e.g., "main..feat", "staged")
    UNIQUE(document_path, content_hash)   -- One session per document+hash combination
);

CREATE INDEX IF NOT EXISTS idx_review_sessions_document_path ON review_sessions(document_path);
CREATE INDEX IF NOT EXISTS idx_review_sessions_hash ON review_sessions(document_path, content_hash);

-- Review comments table
CREATE TABLE IF NOT EXISTS review_comments (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    start_line INTEGER NOT NULL,         -- 1-indexed line number
    end_line INTEGER NOT NULL,           -- Inclusive
    context_text TEXT NOT NULL,          -- Quoted text from document
    comment_text TEXT NOT NULL,          -- User's feedback
    created_at INTEGER NOT NULL,         -- Unix timestamp in nanoseconds
    side TEXT DEFAULT '' NOT NULL CHECK(side IN ('', 'old', 'new')), -- For diffs: 'old' (deletion), 'new' (addition), '' (document)
    FOREIGN KEY (session_id) REFERENCES review_sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_review_comments_session_id ON review_comments(session_id);

-- Schema version 5: Add diff context support
-- Note: ALTER TABLE will fail if columns exist; initSchema handles this by checking version
-- Add session_name and diff_context to review_sessions for diff review sessions
-- Add side column to review_comments to distinguish old lines (deletions) from new lines (additions)
-- Empty string means document review (not a diff comment). Values: 'old' (deletion) or 'new' (addition)
