package audit

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ActionBootstrapAdminCreated = "auth.bootstrap_admin.created"
)

var ErrInvalidEvent = errors.New("invalid audit event")

type Event struct {
	ID          string
	ActorUserID string
	Action      string
	TargetType  string
	TargetID    string
	IPAddress   string
	UserAgent   string
	Metadata    map[string]string
	CreatedAt   time.Time
}

type Store interface {
	Write(ctx context.Context, event Event) error
}

type Writer struct {
	store Store
	now   func() time.Time
	newID func() (string, error)
}

func NewWriter(store Store) Writer {
	return Writer{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
		newID: newUUID,
	}
}

func (w Writer) WithClock(now func() time.Time) Writer {
	w.now = now
	return w
}

func (w Writer) WithIDGenerator(newID func() (string, error)) Writer {
	w.newID = newID
	return w
}

func (w Writer) Record(ctx context.Context, event Event) (Event, error) {
	event.Action = strings.TrimSpace(event.Action)
	event.TargetType = strings.TrimSpace(event.TargetType)
	event.TargetID = strings.TrimSpace(event.TargetID)

	if event.Action == "" || event.TargetType == "" || event.TargetID == "" {
		return Event{}, ErrInvalidEvent
	}
	if event.ID == "" {
		id, err := w.newID()
		if err != nil {
			return Event{}, err
		}
		event.ID = id
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = w.now()
	}
	if event.Metadata == nil {
		event.Metadata = map[string]string{}
	}

	if err := w.store.Write(ctx, event); err != nil {
		return Event{}, err
	}
	return event, nil
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
