-- output_command gains a dedup key: the same (action_id, key) enqueued twice
-- (e.g. on batch replay) must not fire the action a second time. Existing
-- rows get the zero-value '' key, distinguishing them from each other only
-- by action_id until this migration ships (pre-Phase-3 rows are not expected
-- to collide since output_command has had no writers before now).
ALTER TABLE output_command ADD COLUMN key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_output_command_action_key ON output_command(action_id, key);
