package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestEnqueueDefaultsJob(t *testing.T) {
	store := &memoryStore{}
	now := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	queue := NewQueue(store).
		WithClock(func() time.Time { return now }).
		WithIDGenerator(func() (string, error) { return "job-id", nil })

	job, err := queue.Enqueue(context.Background(), EnqueueInput{
		Type: "site.provision",
	})
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if job.ID != "job-id" || job.Status != StatusQueued || job.MaxAttempts != 1 || !job.RunAfter.Equal(now) {
		t.Fatalf("unexpected job defaults: %#v", job)
	}
	if !json.Valid(job.Payload) || string(job.Payload) != "{}" {
		t.Fatalf("unexpected payload: %s", job.Payload)
	}
	if len(store.enqueued) != 1 {
		t.Fatalf("expected one enqueued job, got %d", len(store.enqueued))
	}
}

func TestEnqueueRejectsInvalidJob(t *testing.T) {
	queue := NewQueue(&memoryStore{})
	for _, input := range []EnqueueInput{
		{},
		{Type: "site.provision", Payload: json.RawMessage(`{`)},
	} {
		if _, err := queue.Enqueue(context.Background(), input); !errors.Is(err, ErrInvalidJob) {
			t.Fatalf("expected ErrInvalidJob, got %v", err)
		}
	}
}

func TestFailRetriesUntilMaxAttempts(t *testing.T) {
	store := &memoryStore{}
	now := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	queue := NewQueue(store).WithClock(func() time.Time { return now })

	err := queue.Fail(context.Background(), Job{ID: "job-id", Attempts: 1, MaxAttempts: 3}, FailureInput{
		Message: "temporary failure",
		Backoff: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Fail returned error: %v", err)
	}
	if store.failedFinal || !store.failedRunAfter.Equal(now.Add(5*time.Minute)) || !store.failedFinishedAt.IsZero() {
		t.Fatalf("unexpected retry failure state: %#v", store)
	}
}

func TestFailFinalizesAtMaxAttempts(t *testing.T) {
	store := &memoryStore{}
	now := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	queue := NewQueue(store).WithClock(func() time.Time { return now })

	err := queue.Fail(context.Background(), Job{ID: "job-id", Attempts: 3, MaxAttempts: 3}, FailureInput{
		Message: "final failure",
		Backoff: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Fail returned error: %v", err)
	}
	if !store.failedFinal || !store.failedFinishedAt.Equal(now) {
		t.Fatalf("unexpected final failure state: %#v", store)
	}
}

type memoryStore struct {
	enqueued         []Job
	failedFinal      bool
	failedRunAfter   time.Time
	failedFinishedAt time.Time
}

func (s *memoryStore) Enqueue(_ context.Context, job Job) error {
	s.enqueued = append(s.enqueued, job)
	return nil
}

func (s *memoryStore) ClaimOne(context.Context, time.Time) (Job, error) {
	return Job{}, ErrNoJob
}

func (s *memoryStore) MarkSucceeded(context.Context, string, time.Time) error {
	return nil
}

func (s *memoryStore) MarkFailed(_ context.Context, _ string, final bool, runAfter time.Time, _ time.Time, finishedAt time.Time, _ string) error {
	s.failedFinal = final
	s.failedRunAfter = runAfter
	s.failedFinishedAt = finishedAt
	return nil
}
