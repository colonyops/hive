CREATE TABLE IF NOT EXISTS timers (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL,
    tmux_target   TEXT NOT NULL,
    prompt        TEXT NOT NULL,
    duration_ns   INTEGER NOT NULL,
    fires_at      INTEGER NOT NULL,
    pid           INTEGER,
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    INTEGER NOT NULL,
    fired_at      INTEGER
);

CREATE INDEX IF NOT EXISTS idx_timers_session_status ON timers(session_id, status);
CREATE INDEX IF NOT EXISTS idx_timers_fires_at ON timers(fires_at);
