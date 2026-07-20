-- Deployed builds before this migration could leave confirmation-gated work
-- behind. The deployed-flow model runs queued actions as pending work, so
-- migrate those rows rather than strand them forever.
UPDATE output_command SET status = 'pending' WHERE status = 'awaiting_confirmation';

ALTER TABLE output_command ADD COLUMN result_json TEXT;
ALTER TABLE output_command ADD COLUMN stdout TEXT;
ALTER TABLE output_command ADD COLUMN stderr TEXT;
