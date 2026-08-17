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
	ActionLoginSucceeded        = "auth.login.succeeded"
	ActionLoginFailed           = "auth.login.failed"
	ActionLoginRejected         = "auth.login.rejected"
	ActionLogoutSucceeded       = "auth.logout.succeeded"
	ActionJobRetried            = "job.retried"
	ActionJobCancelled          = "job.cancelled"
	ActionWebServerSelected     = "settings.web_server.selected"
	ActionWebServerChangeDenied = "settings.web_server.change_denied"
	MaxRecentEvents             = 100
	maxActionBytes              = 128
	maxTargetTypeBytes          = 64
	maxTargetIDBytes            = 256
	maxUserAgentBytes           = 2048
	maxMetadataValueBytes       = 1024
	maxMetadataBytes            = 8192
)

var (
	ErrInvalidEvent = errors.New("invalid audit event")
	ErrInvalidLimit = errors.New("invalid audit event limit")
)

type Event struct {
	ID          string            `json:"id"`
	ActorUserID string            `json:"actorUserId,omitempty"`
	Action      string            `json:"action"`
	TargetType  string            `json:"targetType"`
	TargetID    string            `json:"targetId"`
	IPAddress   string            `json:"ipAddress,omitempty"`
	UserAgent   string            `json:"userAgent,omitempty"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"createdAt"`
}

type Store interface {
	Write(ctx context.Context, event Event) error
}

type Reader interface {
	ListRecent(ctx context.Context, limit int) ([]Event, error)
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
	event.ActorUserID = strings.TrimSpace(event.ActorUserID)
	event.Action = strings.TrimSpace(event.Action)
	event.TargetType = strings.TrimSpace(event.TargetType)
	event.TargetID = strings.TrimSpace(event.TargetID)
	event.IPAddress = strings.TrimSpace(event.IPAddress)
	event.UserAgent = strings.TrimSpace(event.UserAgent)

	allowedMetadata, actionKnown := auditMetadataKeys[event.Action]
	if !actionKnown || event.TargetType == "" || event.TargetID == "" ||
		len(event.Action) > maxActionBytes || len(event.TargetType) > maxTargetTypeBytes ||
		len(event.TargetID) > maxTargetIDBytes || len(event.UserAgent) > maxUserAgentBytes {
		return Event{}, ErrInvalidEvent
	}
	if event.Metadata == nil {
		event.Metadata = map[string]string{}
	}
	metadataBytes := 0
	for key, value := range event.Metadata {
		if !allowedMetadata[key] || len(value) > maxMetadataValueBytes {
			return Event{}, ErrInvalidEvent
		}
		metadataBytes += len(key) + len(value)
	}
	if metadataBytes > maxMetadataBytes {
		return Event{}, ErrInvalidEvent
	}
	id, err := w.newID()
	if err != nil {
		return Event{}, err
	}
	event.ID = id
	event.CreatedAt = w.now()

	if err := w.store.Write(ctx, event); err != nil {
		return Event{}, err
	}
	return event, nil
}

var auditMetadataKeys = map[string]map[string]bool{
	ActionBootstrapAdminCreated: {"source": true},
	ActionLoginSucceeded:        {},
	ActionLoginFailed:           {},
	ActionLoginRejected:         {"reason": true},
	ActionLogoutSucceeded:       {},
	ActionJobRetried:            {},
	ActionJobCancelled:          {},
	ActionWebServerSelected:     {"value": true},
	ActionWebServerChangeDenied: {"value": true, "current": true},
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
