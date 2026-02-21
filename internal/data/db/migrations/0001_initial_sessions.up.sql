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
