ALTER TABLE jobs
	ADD COLUMN IF NOT EXISTS retryable BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE jobs
SET retryable = FALSE
WHERE status IN ('succeeded', 'failed', 'cancelled');

CREATE INDEX IF NOT EXISTS jobs_created_at_idx ON jobs (created_at DESC);
CREATE INDEX IF NOT EXISTS job_logs_job_created_idx ON job_logs (job_id, created_at ASC, id ASC);
