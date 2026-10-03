-- Notifications table
CREATE TABLE IF NOT EXISTS notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    level TEXT NOT NULL CHECK(level IN ('info', 'warning', 'error')),
    message TEXT NOT NULL,
    created_at INTEGER NOT NULL -- Unix timestamp in nanoseconds
);

CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at);
