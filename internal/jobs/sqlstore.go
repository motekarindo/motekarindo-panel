package jobs

import (
	"context"
	"database/sql"
	"errors"
	"time"
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
	payload,
	attempts,
	max_attempts,
	run_after,
	created_at,
	updated_at
) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10)`,
		job.ID,
		job.Type,
		job.Status,
		job.ResourceKey,
		string(job.Payload),
		job.Attempts,
		job.MaxAttempts,
		job.RunAfter,
		job.CreatedAt,
		job.UpdatedAt,
	)
	return err
}

func (s SQLStore) ClaimOne(ctx context.Context, now time.Time) (Job, error) {
	row := s.db.QueryRowContext(
		ctx,
		`WITH next AS (
	SELECT id
	FROM jobs candidate
	WHERE candidate.status = 'queued'
		AND candidate.run_after <= $1
		AND (
			candidate.resource_key = ''
			OR NOT EXISTS (
				SELECT 1 FROM jobs running
				WHERE running.status = 'running'
					AND running.resource_key = candidate.resource_key
			)
		)
	ORDER BY candidate.run_after ASC, candidate.created_at ASC
	FOR UPDATE SKIP LOCKED
	LIMIT 1
)
UPDATE jobs
SET status = 'running',
	attempts = attempts + 1,
	started_at = $1,
	updated_at = $1
FROM next
WHERE jobs.id = next.id
RETURNING jobs.id, jobs.type, jobs.status, jobs.resource_key, jobs.payload, jobs.attempts, jobs.max_attempts, jobs.run_after, jobs.started_at, jobs.finished_at, jobs.created_at, jobs.updated_at`,
		now,
	)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	return job, err
}

func (s SQLStore) MarkSucceeded(ctx context.Context, jobID string, finishedAt time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE jobs
SET status = 'succeeded',
	finished_at = $2,
	updated_at = $2
WHERE id = $1`,
		jobID,
		finishedAt,
	)
	return err
}

func (s SQLStore) MarkFailed(ctx context.Context, jobID string, final bool, runAfter time.Time, updatedAt time.Time, finishedAt time.Time, message string) error {
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

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE jobs
SET status = $2,
	run_after = $3,
	finished_at = $4,
	updated_at = $5
WHERE id = $1`,
		jobID,
		status,
		runAfter,
		finished,
		updatedAt,
	); err != nil {
		return err
	}

	if message != "" {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO job_logs (job_id, level, message, created_at)
VALUES ($1, 'error', $2, $3)`,
			jobID,
			message,
			updatedAt,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

type jobScanner interface {
	Scan(dest ...any) error
}

func scanJob(row jobScanner) (Job, error) {
	var job Job
	var payload []byte
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	err := row.Scan(
		&job.ID,
		&job.Type,
		&job.Status,
		&job.ResourceKey,
		&payload,
		&job.Attempts,
		&job.MaxAttempts,
		&job.RunAfter,
		&startedAt,
		&finishedAt,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return Job{}, err
	}
	job.Payload = payload
	if startedAt.Valid {
		job.StartedAt = startedAt.Time
	}
	if finishedAt.Valid {
		job.FinishedAt = finishedAt.Time
	}
	return job, nil
}
