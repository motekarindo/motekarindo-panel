package jobs

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

var (
	ErrInvalidJob = errors.New("invalid job")
	ErrNoJob      = errors.New("no job available")
)

type Job struct {
	ID          string
	Type        string
	Status      Status
	ResourceKey string
	Payload     json.RawMessage
	Attempts    int
	MaxAttempts int
	RunAfter    time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type EnqueueInput struct {
	Type        string
	ResourceKey string
	Payload     json.RawMessage
	MaxAttempts int
	RunAfter    time.Time
}

type FailureInput struct {
	JobID   string
	Message string
	Backoff time.Duration
}

type Store interface {
	Enqueue(ctx context.Context, job Job) error
	ClaimOne(ctx context.Context, now time.Time) (Job, error)
	MarkSucceeded(ctx context.Context, jobID string, finishedAt time.Time) error
	MarkFailed(ctx context.Context, jobID string, final bool, runAfter time.Time, updatedAt time.Time, finishedAt time.Time, message string) error
}

type Queue struct {
	store Store
	now   func() time.Time
	newID func() (string, error)
}

func NewQueue(store Store) Queue {
	return Queue{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
		newID: newUUID,
	}
}

func (q Queue) WithClock(now func() time.Time) Queue {
	q.now = now
	return q
}

func (q Queue) WithIDGenerator(newID func() (string, error)) Queue {
	q.newID = newID
	return q
}

func (q Queue) Enqueue(ctx context.Context, input EnqueueInput) (Job, error) {
	input.Type = strings.TrimSpace(input.Type)
	if input.Type == "" {
		return Job{}, ErrInvalidJob
	}
	if len(input.Payload) == 0 {
		input.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(input.Payload) {
		return Job{}, ErrInvalidJob
	}
	if input.MaxAttempts <= 0 {
		input.MaxAttempts = 1
	}
	now := q.now()
	if input.RunAfter.IsZero() {
		input.RunAfter = now
	}
	id, err := q.newID()
	if err != nil {
		return Job{}, err
	}

	job := Job{
		ID:          id,
		Type:        input.Type,
		Status:      StatusQueued,
		ResourceKey: strings.TrimSpace(input.ResourceKey),
		Payload:     input.Payload,
		MaxAttempts: input.MaxAttempts,
		RunAfter:    input.RunAfter,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := q.store.Enqueue(ctx, job); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (q Queue) ClaimOne(ctx context.Context) (Job, error) {
	return q.store.ClaimOne(ctx, q.now())
}

func (q Queue) Succeed(ctx context.Context, jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return ErrInvalidJob
	}
	return q.store.MarkSucceeded(ctx, jobID, q.now())
}

func (q Queue) Fail(ctx context.Context, job Job, input FailureInput) error {
	if strings.TrimSpace(job.ID) == "" {
		return ErrInvalidJob
	}
	now := q.now()
	final := job.Attempts >= job.MaxAttempts
	runAfter := now
	finishedAt := now
	if !final {
		runAfter = now.Add(input.Backoff)
		finishedAt = time.Time{}
	}
	return q.store.MarkFailed(ctx, job.ID, final, runAfter, now, finishedAt, input.Message)
}

func newUUID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}
