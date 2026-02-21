-- Review sessions table
CREATE TABLE IF NOT EXISTS review_sessions (
    id TEXT PRIMARY KEY,
    document_path TEXT NOT NULL,          -- Absolute path to document
    content_hash TEXT NOT NULL,           -- SHA256 hash of document content
    created_at INTEGER NOT NULL,          -- Unix timestamp in nanoseconds
    finalized_at INTEGER,                 -- Unix timestamp in nanoseconds, NULL if not finalized
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
    FOREIGN KEY (session_id) REFERENCES review_sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_review_comments_session_id ON review_comments(session_id);
