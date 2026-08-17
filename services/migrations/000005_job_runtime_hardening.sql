ALTER TABLE jobs
	ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
	ADD COLUMN IF NOT EXISTS claim_token UUID,
	ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ,
	ADD COLUMN IF NOT EXISTS result_code TEXT,
	ADD COLUMN IF NOT EXISTS result JSONB;

UPDATE jobs
SET idempotency_key = id::text
WHERE idempotency_key IS NULL;

INSERT INTO job_logs (job_id, level, message, created_at)
SELECT id,
	'error',
	CASE
		WHEN attempts >= max_attempts THEN 'legacy running job failed during lease migration after final attempt'
		ELSE 'legacy running job requeued during lease migration'
	END,
	now()
FROM jobs
WHERE status = 'running';

UPDATE jobs
SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
	started_at = CASE WHEN attempts >= max_attempts THEN started_at ELSE NULL END,
	finished_at = CASE WHEN attempts >= max_attempts THEN now() ELSE NULL END,
	run_after = LEAST(run_after, now()),
	updated_at = now()
WHERE status = 'running';

ALTER TABLE jobs
	ALTER COLUMN idempotency_key SET NOT NULL,
	ADD CONSTRAINT jobs_type_length_check CHECK (octet_length(type) BETWEEN 1 AND 128),
	ADD CONSTRAINT jobs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'retrying')),
	ADD CONSTRAINT jobs_resource_key_length_check CHECK (octet_length(resource_key) <= 512),
	ADD CONSTRAINT jobs_idempotency_key_length_check CHECK (octet_length(idempotency_key) BETWEEN 1 AND 128),
	ADD CONSTRAINT jobs_attempts_check CHECK (attempts BETWEEN 0 AND 25),
	ADD CONSTRAINT jobs_max_attempts_check CHECK (max_attempts BETWEEN 1 AND 25),
	ADD CONSTRAINT jobs_attempt_limit_check CHECK (attempts <= max_attempts),
	ADD CONSTRAINT jobs_payload_object_check CHECK (jsonb_typeof(payload) = 'object'),
	ADD CONSTRAINT jobs_payload_size_check CHECK (octet_length(payload::text) <= 65536),
	ADD CONSTRAINT jobs_result_code_length_check CHECK (result_code IS NULL OR octet_length(result_code) BETWEEN 1 AND 64),
	ADD CONSTRAINT jobs_result_size_check CHECK (result IS NULL OR octet_length(result::text) <= 65536),
	ADD CONSTRAINT jobs_claim_lease_check CHECK (
		(status = 'running' AND claim_token IS NOT NULL AND lease_expires_at IS NOT NULL)
		OR (status <> 'running' AND claim_token IS NULL AND lease_expires_at IS NULL)
	);

CREATE UNIQUE INDEX IF NOT EXISTS jobs_idempotency_key_idx ON jobs (idempotency_key);
CREATE INDEX IF NOT EXISTS jobs_running_lease_idx ON jobs (lease_expires_at) WHERE status = 'running';

ALTER TABLE job_logs
	ADD CONSTRAINT job_logs_level_check CHECK (level IN ('debug', 'info', 'warn', 'error')),
	ADD CONSTRAINT job_logs_message_length_check CHECK (char_length(message) <= 1024);
