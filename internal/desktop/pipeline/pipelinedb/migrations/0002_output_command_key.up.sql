-- output_command gains a dedup key: the same (action_id, key) enqueued twice
-- (e.g. on batch replay) must not fire the action a second time. Existing
-- rows get the zero-value '' key, distinguishing them from each other only
-- by action_id. This pipeline database had no output_command writers before
-- the dedup-key migration, so collisions are not expected.
ALTER TABLE output_command ADD COLUMN key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_output_command_action_key ON output_command(action_id, key);
