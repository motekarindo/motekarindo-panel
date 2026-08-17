package jobs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxJobPayloadBytes   = 64 << 10
	maxJobTypeBytes      = 128
	maxResourceKeyBytes  = 512
	maxIdempotencyBytes  = 128
	maxJobAttempts       = 25
	defaultLeaseDuration = 10 * time.Minute
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

var (
	ErrInvalidJob        = errors.New("invalid job")
	ErrNoJob             = errors.New("no job available")
	ErrJobNotFound       = errors.New("job not found")
	ErrInvalidTransition = errors.New("invalid job state transition")
)

type Job struct {
	ID             string
	Type           string
	Status         Status
	ResourceKey    string
	IdempotencyKey string
	Payload        json.RawMessage
	Attempts       int
	MaxAttempts    int
	Retryable      bool
	ClaimToken     string
	RunAfter       time.Time
	LeaseExpiresAt time.Time
	StartedAt      time.Time
	FinishedAt     time.Time
	ResultCode     string
	Result         json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type EnqueueInput struct {
	Type           string
	ResourceKey    string
	IdempotencyKey string
	Payload        json.RawMessage
	MaxAttempts    int
	RunAfter       time.Time
}

type FailureInput struct {
	Message   string
	Backoff   time.Duration
	Permanent bool
}

type Result struct {
	Code string
	Data json.RawMessage
	Logs []Log
}

type Log struct {
	ID        int64
	Level     string
	Message   string
	CreatedAt time.Time
}

type Mutation struct {
	ActorUserID string
	IPAddress   string
	UserAgent   string
}

type Store interface {
	Enqueue(ctx context.Context, job Job) error
	ClaimOne(ctx context.Context, now time.Time, leaseExpiresAt time.Time) (Job, error)
	MarkSucceeded(ctx context.Context, job Job, finishedAt time.Time, result Result) error
	MarkFailed(ctx context.Context, job Job, final bool, retryable bool, runAfter time.Time, updatedAt time.Time, finishedAt time.Time, message string) error
}

type Queue struct {
	store         Store
	now           func() time.Time
	newID         func() (string, error)
	leaseDuration time.Duration
}

func NewQueue(store Store) Queue {
	return Queue{
		store:         store,
		now:           func() time.Time { return time.Now().UTC() },
		newID:         newUUID,
		leaseDuration: defaultLeaseDuration,
	}
}

func (q Queue) WithLeaseDuration(duration time.Duration) Queue {
	if duration > 0 {
		q.leaseDuration = duration
	}
	return q
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
	input.ResourceKey = strings.TrimSpace(input.ResourceKey)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Type == "" || len(input.Type) > maxJobTypeBytes || len(input.ResourceKey) > maxResourceKeyBytes || len(input.IdempotencyKey) > maxIdempotencyBytes {
		return Job{}, ErrInvalidJob
	}
	if len(input.Payload) == 0 {
		input.Payload = json.RawMessage(`{}`)
	}
	input.Payload = bytes.TrimSpace(input.Payload)
	if len(input.Payload) == 0 || len(input.Payload) > maxJobPayloadBytes || input.Payload[0] != '{' || !json.Valid(input.Payload) {
		return Job{}, ErrInvalidJob
	}
	input.Payload = append(json.RawMessage(nil), input.Payload...)
	if input.MaxAttempts <= 0 {
		input.MaxAttempts = 1
	}
	if input.MaxAttempts > maxJobAttempts {
		return Job{}, ErrInvalidJob
	}
	now := q.now()
	if input.RunAfter.IsZero() {
		input.RunAfter = now
	}
	id, err := q.newID()
	if err != nil {
		return Job{}, err
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = id
	}

	job := Job{
		ID:             id,
		Type:           input.Type,
		Status:         StatusQueued,
		ResourceKey:    input.ResourceKey,
		IdempotencyKey: input.IdempotencyKey,
		Payload:        input.Payload,
		MaxAttempts:    input.MaxAttempts,
		Retryable:      true,
		RunAfter:       input.RunAfter,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := q.store.Enqueue(ctx, job); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (q Queue) ClaimOne(ctx context.Context) (Job, error) {
	now := q.now()
	return q.store.ClaimOne(ctx, now, now.Add(q.leaseDuration))
}

func (q Queue) Succeed(ctx context.Context, job Job, result Result) error {
	if strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.ClaimToken) == "" {
		return ErrInvalidJob
	}
	result, err := normalizeResult(result)
	if err != nil {
		return err
	}
	return q.store.MarkSucceeded(ctx, job, q.now(), result)
}

func (q Queue) Fail(ctx context.Context, job Job, input FailureInput) error {
	if strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.ClaimToken) == "" || input.Backoff < 0 {
		return ErrInvalidJob
	}
	if strings.TrimSpace(input.Message) != "" {
		input.Message = safeFailureMessage(errors.New(input.Message))
	}
	now := q.now()
	final := input.Permanent || job.Attempts >= job.MaxAttempts
	runAfter := now
	finishedAt := now
	if !final {
		runAfter = now.Add(input.Backoff)
		finishedAt = time.Time{}
	}
	return q.store.MarkFailed(ctx, job, final, !input.Permanent, runAfter, now, finishedAt, input.Message)
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

func validJobID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for index, value := range []byte(id) {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if value != '-' {
				return false
			}
			continue
		}
		if !((value >= '0' && value <= '9') || (value >= 'a' && value <= 'f') || (value >= 'A' && value <= 'F')) {
			return false
		}
	}
	return true
}
