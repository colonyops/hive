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

-- The log replaces the bounded stream columns. Output recorded before it
-- moves across as one row per stream, so an older run still shows it.
INSERT INTO action_run_log (command_id, attempt, stream, text, created_at)
SELECT id, MAX(attempts, 1), 'stdout', stdout, created_at FROM output_command
WHERE COALESCE(stdout, '') <> '';

INSERT INTO action_run_log (command_id, attempt, stream, text, created_at)
SELECT id, MAX(attempts, 1), 'stderr', stderr, created_at FROM output_command
WHERE COALESCE(stderr, '') <> '';

ALTER TABLE output_command DROP COLUMN stdout;
ALTER TABLE output_command DROP COLUMN stderr;

ALTER TABLE output_command ADD COLUMN finished_at INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_job_command_id ON job(command_id);
