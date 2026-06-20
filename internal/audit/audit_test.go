package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRecordFillsDefaultsAndWritesEvent(t *testing.T) {
	store := &memoryStore{}
	now := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	writer := NewWriter(store).
		WithClock(func() time.Time { return now }).
		WithIDGenerator(func() (string, error) { return "event-id", nil })

	event, err := writer.Record(context.Background(), Event{
		Action:     " auth.bootstrap_admin.created ",
		TargetType: " user ",
		TargetID:   " admin-id ",
		Metadata:   map[string]string{"email": "owner@example.com"},
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	if event.ID != "event-id" || !event.CreatedAt.Equal(now) {
		t.Fatalf("unexpected defaults: %#v", event)
	}
	if event.Action != ActionBootstrapAdminCreated || event.TargetType != "user" || event.TargetID != "admin-id" {
		t.Fatalf("unexpected normalized event: %#v", event)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected one written event, got %d", len(store.events))
	}
}

func TestRecordRejectsMissingRequiredFields(t *testing.T) {
	writer := NewWriter(&memoryStore{})

	_, err := writer.Record(context.Background(), Event{Action: "something"})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected ErrInvalidEvent, got %v", err)
	}
}

func TestRecordReturnsStoreError(t *testing.T) {
	want := errors.New("write failed")
	writer := NewWriter(&memoryStore{err: want})

	_, err := writer.Record(context.Background(), Event{
		Action:     ActionBootstrapAdminCreated,
		TargetType: "user",
		TargetID:   "admin-id",
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected store error, got %v", err)
	}
}

type memoryStore struct {
	events []Event
	err    error
}

func (s *memoryStore) Write(_ context.Context, event Event) error {
	if s.err != nil {
		return s.err
	}
	s.events = append(s.events, event)
	return nil
}
