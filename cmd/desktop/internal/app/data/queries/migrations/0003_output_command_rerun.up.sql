-- Preserve the original deduplicated command for flow replay while allowing
-- explicitly confirmed manual reruns to retain their own execution history.
ALTER TABLE output_command ADD COLUMN is_rerun INTEGER NOT NULL DEFAULT 0;

DROP INDEX idx_output_command_action_key;
CREATE UNIQUE INDEX idx_output_command_action_key
ON output_command(action_id, key)
WHERE is_rerun = 0;
