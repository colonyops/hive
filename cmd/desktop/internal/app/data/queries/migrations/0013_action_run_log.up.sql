-- The output of each attempt of an output command, one row per line. Rows go
-- with their command, so command retention bounds the log as well.
CREATE TABLE action_run_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    command_id INTEGER NOT NULL REFERENCES output_command(id) ON DELETE CASCADE,
    attempt    INTEGER NOT NULL,
    stream     TEXT NOT NULL CHECK (stream IN ('stdout', 'stderr', 'system')),
    text       TEXT NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_action_run_log_command ON action_run_log(command_id, id);

ALTER TABLE output_command ADD COLUMN finished_at INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_job_command_id ON job(command_id);
