package jobs

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/motekar/motekar-panel/internal/audit"
)

type SQLStore struct {
	db *sql.DB
}

func NewSQLStore(db *sql.DB) SQLStore {
	return SQLStore{db: db}
}

func (s SQLStore) Enqueue(ctx context.Context, job Job) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO jobs (
	id,
	type,
	status,
	resource_key,
	idempotency_key,
	payload,
	attempts,
	max_attempts,
	run_after,
	created_at,
	updated_at
) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11)`,
		job.ID,
		job.Type,
		job.Status,
		job.ResourceKey,
		job.IdempotencyKey,
		string(job.Payload),
		job.Attempts,
		job.MaxAttempts,
		job.RunAfter,
		job.CreatedAt,
		job.UpdatedAt,
	)
	return err
}

func (s SQLStore) ClaimOne(ctx context.Context, now time.Time, leaseExpiresAt time.Time) (Job, error) {
	row := s.db.QueryRowContext(
		ctx,
		`WITH expired_candidates AS MATERIALIZED (
	SELECT id
	FROM jobs
	WHERE status = 'running'
		AND lease_expires_at <= $1
		AND attempts >= max_attempts
	ORDER BY lease_expires_at ASC
	FOR UPDATE SKIP LOCKED
	LIMIT 100
), expired AS (
	UPDATE jobs
	SET status = 'failed',
		retryable = TRUE,
		claim_token = NULL,
		lease_expires_at = NULL,
		finished_at = $1,
		updated_at = $1
	FROM expired_candidates
	WHERE jobs.id = expired_candidates.id
	RETURNING jobs.id
), expired_logs AS (
	INSERT INTO job_logs (job_id, level, message, created_at)
	SELECT id, 'error', 'job lease expired after final attempt', $1
	FROM expired
), next_candidate AS MATERIALIZED (
	SELECT id, resource_key
	FROM jobs candidate
	WHERE (
			(candidate.status = 'queued' AND candidate.run_after <= $1 AND candidate.attempts < candidate.max_attempts)
			OR (
				candidate.status = 'running'
				AND candidate.lease_expires_at <= $1
				AND candidate.attempts < candidate.max_attempts
			)
		)
		AND (
			candidate.resource_key = ''
			OR NOT EXISTS (
				SELECT 1 FROM jobs running
				WHERE running.status = 'running'
					AND running.lease_expires_at > $1
					AND running.resource_key = candidate.resource_key
					AND running.id <> candidate.id
			)
		)
	ORDER BY candidate.run_after ASC, candidate.created_at ASC
	FOR UPDATE SKIP LOCKED
	LIMIT 1
), next AS (
	SELECT id
	FROM next_candidate
	WHERE resource_key = ''
		OR pg_try_advisory_xact_lock(hashtextextended(resource_key, 0))
)
UPDATE jobs
SET status = 'running',
	attempts = attempts + 1,
	claim_token = gen_random_uuid(),
	lease_expires_at = $2,
	started_at = $1,
	finished_at = NULL,
	updated_at = $1
FROM next
WHERE jobs.id = next.id
RETURNING jobs.id, jobs.type, jobs.status, jobs.resource_key, jobs.idempotency_key, jobs.payload, jobs.attempts, jobs.max_attempts, jobs.retryable, jobs.claim_token, jobs.run_after, jobs.lease_expires_at, jobs.started_at, jobs.finished_at, jobs.result_code, jobs.result, jobs.created_at, jobs.updated_at`,
		now,
		leaseExpiresAt,
	)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	return job, err
}

func (s SQLStore) MarkSucceeded(ctx context.Context, job Job, finishedAt time.Time, executionResult Result) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var resultData any
	if len(executionResult.Data) > 0 {
		resultData = string(executionResult.Data)
	}
	updateResult, err := tx.ExecContext(
		ctx,
		`UPDATE jobs
SET status = 'succeeded',
	retryable = FALSE,
	claim_token = NULL,
	lease_expires_at = NULL,
	result_code = $3,
	result = $4::jsonb,
	finished_at = $5,
	updated_at = $5
WHERE id = $1
		AND status = 'running'
		AND claim_token = $2`,
		job.ID,
		job.ClaimToken,
		executionResult.Code,
		resultData,
		finishedAt,
	)
	if err != nil {
		return err
	}
	if err := requireUpdatedRow(updateResult); err != nil {
		return err
	}
	if err := insertLogs(ctx, tx, job.ID, executionResult.Logs, finishedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s SQLStore) MarkFailed(ctx context.Context, job Job, final bool, retryable bool, runAfter time.Time, updatedAt time.Time, finishedAt time.Time, message string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	status := StatusQueued
	var finished any
	if final {
		status = StatusFailed
		finished = finishedAt
	}

	result, err := tx.ExecContext(
		ctx,
		`UPDATE jobs
SET status = $2,
	run_after = $3,
	finished_at = $4,
	retryable = $7,
	claim_token = NULL,
	lease_expires_at = NULL,
	started_at = CASE WHEN $2 = 'queued' THEN NULL ELSE started_at END,
	updated_at = $5
WHERE id = $1
		AND status = 'running'
		AND claim_token = $6`,
		job.ID,
		status,
		runAfter,
		finished,
		updatedAt,
		job.ClaimToken,
		retryable,
	)
	if err != nil {
		return err
	}
	if err := requireUpdatedRow(result); err != nil {
		return err
	}

	if message != "" {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO job_logs (job_id, level, message, created_at)
VALUES ($1, 'error', $2, $3)`,
			job.ID,
			message,
			updatedAt,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s SQLStore) ListRecent(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 || limit > 100 {
		return nil, ErrInvalidJob
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, type, status, resource_key, idempotency_key, payload, attempts, max_attempts, retryable,
	claim_token, run_after, lease_expires_at, started_at, finished_at, result_code, result, created_at, updated_at
FROM jobs
ORDER BY created_at DESC, id DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

func (s SQLStore) Get(ctx context.Context, id string) (Job, error) {
	if !validJobID(id) {
		return Job{}, ErrJobNotFound
	}
	job, err := scanJob(s.db.QueryRowContext(ctx, `
SELECT id, type, status, resource_key, idempotency_key, payload, attempts, max_attempts, retryable,
	claim_token, run_after, lease_expires_at, started_at, finished_at, result_code, result, created_at, updated_at
FROM jobs
WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	return job, err
}

func (s SQLStore) ListLogs(ctx context.Context, jobID string, limit int) ([]Log, error) {
	if !validJobID(jobID) || limit <= 0 || limit > 500 {
		return nil, ErrInvalidJob
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, level, message, created_at
FROM job_logs
WHERE job_id = $1
ORDER BY created_at ASC, id ASC
LIMIT $2`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]Log, 0)
	for rows.Next() {
		var log Log
		if err := rows.Scan(&log.ID, &log.Level, &log.Message, &log.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (s SQLStore) Retry(ctx context.Context, id string, mutation Mutation) error {
	return s.mutate(ctx, id, mutation, StatusFailed, StatusQueued, audit.ActionJobRetried)
}

func (s SQLStore) Cancel(ctx context.Context, id string, mutation Mutation) error {
	return s.mutate(ctx, id, mutation, StatusQueued, StatusCancelled, audit.ActionJobCancelled)
}

func (s SQLStore) mutate(ctx context.Context, id string, mutation Mutation, from Status, to Status, action string) error {
	if !validJobID(id) {
		return ErrJobNotFound
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status Status
	var retryable bool
	if err := tx.QueryRowContext(ctx, `SELECT status, retryable FROM jobs WHERE id = $1 FOR UPDATE`, id).Scan(&status, &retryable); errors.Is(err, sql.ErrNoRows) {
		return ErrJobNotFound
	} else if err != nil {
		return err
	}
	if status != from || (action == audit.ActionJobRetried && !retryable) {
		return ErrInvalidTransition
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
UPDATE jobs
SET status = $2,
	attempts = CASE WHEN $2 = 'queued' THEN 0 ELSE attempts END,
	retryable = CASE WHEN $2 = 'queued' THEN TRUE ELSE FALSE END,
	run_after = CASE WHEN $2 = 'queued' THEN $3 ELSE run_after END,
	started_at = NULL,
	finished_at = CASE WHEN $2 = 'cancelled' THEN $3 ELSE NULL END,
	result_code = NULL,
	result = NULL,
	updated_at = $3
WHERE id = $1`, id, to, now); err != nil {
		return err
	}

	if _, err := audit.NewWriter(audit.NewSQLStore(tx)).Record(ctx, audit.Event{
		ActorUserID: mutation.ActorUserID,
		Action:      action,
		TargetType:  "job",
		TargetID:    id,
		IPAddress:   mutation.IPAddress,
		UserAgent:   mutation.UserAgent,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func insertLogs(ctx context.Context, tx *sql.Tx, jobID string, logs []Log, createdAt time.Time) error {
	for _, log := range logs {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO job_logs (job_id, level, message, created_at) VALUES ($1, $2, $3, $4)`,
			jobID,
			log.Level,
			log.Message,
			createdAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func requireUpdatedRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrInvalidTransition
	}
	return nil
}

type jobScanner interface {
	Scan(dest ...any) error
}

func scanJob(row jobScanner) (Job, error) {
	var job Job
	var payload []byte
	var result []byte
	var claimToken sql.NullString
	var resultCode sql.NullString
	var leaseExpiresAt sql.NullTime
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	err := row.Scan(
		&job.ID,
		&job.Type,
		&job.Status,
		&job.ResourceKey,
		&job.IdempotencyKey,
		&payload,
		&job.Attempts,
		&job.MaxAttempts,
		&job.Retryable,
		&claimToken,
		&job.RunAfter,
		&leaseExpiresAt,
		&startedAt,
		&finishedAt,
		&resultCode,
		&result,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return Job{}, err
	}
	job.Payload = payload
	job.Result = result
	if resultCode.Valid {
		job.ResultCode = resultCode.String
	}
	if claimToken.Valid {
		job.ClaimToken = claimToken.String
	}
	if leaseExpiresAt.Valid {
		job.LeaseExpiresAt = leaseExpiresAt.Time
	}
	if startedAt.Valid {
		job.StartedAt = startedAt.Time
	}
	if finishedAt.Valid {
		job.FinishedAt = finishedAt.Time
	}
	return job, nil
}
