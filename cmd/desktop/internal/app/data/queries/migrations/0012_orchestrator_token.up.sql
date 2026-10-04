-- Access tokens for clients outside an agent workspace that call the
-- hive-orchestrator MCP server. Only a SHA-256 of each token is kept: the
-- plaintext is shown once, when the token is created.
CREATE TABLE orchestrator_token (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    token_hash   TEXT NOT NULL UNIQUE,
    hint         TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER
) STRICT;
