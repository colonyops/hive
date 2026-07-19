-- output_command gains bounded-retry bookkeeping: attempts (how many times
-- the output worker has tried and failed to execute this command) and
-- last_error (the most recent failure, kept for diagnosis even after the
-- command is marked failed for good).
ALTER TABLE output_command ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE output_command ADD COLUMN last_error TEXT;
