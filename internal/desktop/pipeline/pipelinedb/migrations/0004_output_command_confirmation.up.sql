-- Manual action commands are moved out of the runnable queue until the
-- corresponding action is configured for automatic execution or a future
-- confirmation UI applies them. Keep runnable rows efficiently ordered by
-- their monotonic primary key.
CREATE INDEX idx_output_command_status_id ON output_command(status, id);
