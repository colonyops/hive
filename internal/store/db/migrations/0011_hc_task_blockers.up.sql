CREATE TABLE IF NOT EXISTS hc_task_blockers (
    blocker_id TEXT NOT NULL,
    blocked_id TEXT NOT NULL,
    PRIMARY KEY (blocker_id, blocked_id),
    FOREIGN KEY (blocker_id) REFERENCES hc_items(id) ON DELETE CASCADE,
    FOREIGN KEY (blocked_id) REFERENCES hc_items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_hc_blockers_blocker ON hc_task_blockers(blocker_id);
CREATE INDEX IF NOT EXISTS idx_hc_blockers_blocked ON hc_task_blockers(blocked_id);
