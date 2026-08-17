package audit

import (
	"context"
	"errors"
	"strings"
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
		ID:         "caller-supplied-id",
		Action:     " auth.bootstrap_admin.created ",
		TargetType: " user ",
		TargetID:   " admin-id ",
		Metadata:   map[string]string{"source": "bootstrap"},
		CreatedAt:  now.Add(-time.Hour),
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

func TestRecordRejectsUnknownActionsAndMetadata(t *testing.T) {
	t.Parallel()

	writer := NewWriter(&memoryStore{})
	for _, event := range []Event{
		{Action: "unknown.action", TargetType: "user", TargetID: "user-id"},
		{Action: ActionLoginRejected, TargetType: "authentication", TargetID: "login", Metadata: map[string]string{"password": "secret"}},
	} {
		if _, err := writer.Record(context.Background(), event); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("Record(%#v) error = %v, want %v", event, err, ErrInvalidEvent)
		}
	}
}

func TestDecodeMetadataHandlesNonStringJSONValues(t *testing.T) {
	t.Parallel()

	metadata, err := decodeMetadata([]byte(`{"attempt":1,"details":{"source":"agent"},"outcome":"denied"}`))
	if err != nil {
		t.Fatalf("decodeMetadata: %v", err)
	}
	if metadata["attempt"] != "1" || metadata["details"] != `{"source":"agent"}` || metadata["outcome"] != "denied" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestListRecentRejectsInvalidLimitsBeforeQuery(t *testing.T) {
	t.Parallel()

	store := SQLStore{}
	for _, limit := range []int{0, -1, MaxRecentEvents + 1} {
		if _, err := store.ListRecent(context.Background(), limit); !errors.Is(err, ErrInvalidLimit) {
			t.Fatalf("ListRecent(%d) error = %v, want %v", limit, err, ErrInvalidLimit)
		}
	}
}

func TestRecordEnforcesFieldSizeBoundaries(t *testing.T) {
	t.Parallel()

	writer := NewWriter(&memoryStore{})
	validUserAgent := strings.Repeat("é", maxUserAgentBytes/2)
	if _, err := writer.Record(context.Background(), Event{
		Action:     ActionLoginRejected,
		TargetType: "authentication",
		TargetID:   strings.Repeat("a", maxTargetIDBytes),
		UserAgent:  validUserAgent,
		Metadata:   map[string]string{"reason": strings.Repeat("r", maxMetadataValueBytes)},
	}); err != nil {
		t.Fatalf("Record boundary event: %v", err)
	}

	for _, event := range []Event{
		{Action: ActionLoginRejected, TargetType: "authentication", TargetID: strings.Repeat("a", maxTargetIDBytes+1)},
		{Action: ActionLoginRejected, TargetType: "authentication", TargetID: "login", UserAgent: validUserAgent + "é"},
		{Action: ActionLoginRejected, TargetType: "authentication", TargetID: "login", Metadata: map[string]string{"reason": strings.Repeat("r", maxMetadataValueBytes+1)}},
	} {
		if _, err := writer.Record(context.Background(), event); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("Record over-limit event error = %v, want %v", err, ErrInvalidEvent)
		}
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
