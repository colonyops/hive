-- Retention keeps only bounded diagnostic history. Event-log safety is
-- enforced by consumer offsets in application code; these indexes support the
-- bounded history pruning and the canvas's recent-runs query.
CREATE INDEX idx_node_run_ended_at ON node_run(ended_at DESC);
CREATE INDEX idx_node_run_flow_ended_at ON node_run(flow_id, ended_at DESC);

-- Only completed commands are eligible for retention. Pending, awaiting
-- confirmation, running, and retryable commands remain durable regardless of
-- age, so this partial index is both smaller and aligned with the prune.
CREATE INDEX idx_output_command_terminal_id ON output_command(id DESC)
WHERE status IN ('done', 'failed');
