ALTER TABLE output_command ADD COLUMN dispatch_lane TEXT NOT NULL DEFAULT 'automatic';
ALTER TABLE output_command ADD COLUMN claim_token TEXT NOT NULL DEFAULT '';
ALTER TABLE output_command ADD COLUMN claimed_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE output_command ADD COLUMN not_before INTEGER NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX idx_output_command_active_action_key
ON output_command(action_id, key)
WHERE status IN ('pending', 'running');
