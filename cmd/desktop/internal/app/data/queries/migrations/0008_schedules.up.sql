-- How far each workspace schedule has been evaluated, so a run the app
-- missed (it was closed when due) is still detected the next time it
-- evaluates. cron is snapshotted alongside the cursor: a re-timed schedule's
-- cron no longer matches, which the scheduler reads as "new" rather than
-- back-filling occurrences under the old cadence.
CREATE TABLE schedule_cursor (
    workspace          TEXT NOT NULL,
    schedule_id        TEXT NOT NULL,
    evaluated_through  INTEGER NOT NULL,
    cron               TEXT NOT NULL,
    PRIMARY KEY (workspace, schedule_id)
) STRICT;

-- One schedule execution attempt. session_id is NULL when the run launched
-- no chat (a failed or skipped run).
CREATE TABLE schedule_run (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace     TEXT NOT NULL,
    schedule_id   TEXT NOT NULL,
    schedule_name TEXT NOT NULL,
    scheduled_for INTEGER NOT NULL,
    started_at    INTEGER NOT NULL,
    reason        TEXT NOT NULL,
    missed        INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL,
    session_id    INTEGER,
    prompt        TEXT NOT NULL,
    error         TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE INDEX schedule_run_by_schedule ON schedule_run (workspace, schedule_id, started_at DESC);

-- A chat a schedule started names its schedule here rather than through
-- schedule_run: session ids are reused after a delete and runs are pruned, so
-- a derivation would mislabel or lose it.
ALTER TABLE agent_workspace_session ADD COLUMN schedule_id TEXT NOT NULL DEFAULT '';

-- The capability a launch hands its own process: presenting it is what lets a
-- chat end its own session without the frontend's terminal token.
ALTER TABLE agent_workspace_session ADD COLUMN end_token TEXT NOT NULL DEFAULT '';
