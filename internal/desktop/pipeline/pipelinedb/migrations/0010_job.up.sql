-- job is the desktop's live action-run lifecycle, surfaced by the titlebar
-- spinner chip and its popover: queued -> running -> done|failed. Unlike
-- activity_event (a terminal audit log), a job row is written at every
-- transition so the frontend can show in-flight work. status stays plain TEXT
-- (the jobs package owns the typed JobStatus and converts at its boundary).
-- created_at/updated_at are unix MILLISECONDS (like activity_event) so they
-- survive the int64->JS number boundary in the Wails bindings. command_id is a
-- nullable link to the originating output_command row so the popover can
-- deep-link to the existing ActionRun detail. step is a human label derived per
-- status transition in v1; the column exists so executor sub-steps can be added
-- later without a schema change.
CREATE TABLE job (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    status      TEXT NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    step        TEXT NOT NULL DEFAULT '',
    action_id   TEXT NOT NULL DEFAULT '',
    target      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    command_id  INTEGER
) STRICT;
