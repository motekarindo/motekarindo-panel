package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/motekar/motekar-panel/internal/audit"
)

func TestParseWebServerAcceptsSupportedValues(t *testing.T) {
	for _, input := range []string{"nginx", " NGINX ", "apache", "Apache"} {
		if _, err := ParseWebServer(input); err != nil {
			t.Fatalf("ParseWebServer(%q) returned error: %v", input, err)
		}
	}
}

func TestParseWebServerRejectsUnsupportedValue(t *testing.T) {
	if _, err := ParseWebServer("caddy"); !errors.Is(err, ErrUnsupportedWebServer) {
		t.Fatalf("expected ErrUnsupportedWebServer, got %v", err)
	}
}

func TestSelectPersistsImmutableWebServerOnce(t *testing.T) {
	store := newMemoryStore()
	service := NewWebServerService(store)

	selected, err := service.Select(context.Background(), "nginx")
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if selected != WebServerNginx {
		t.Fatalf("expected nginx, got %q", selected)
	}

	setting, err := store.Get(context.Background(), WebServerSettingKey)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if setting.Value != "nginx" || !setting.IsImmutable {
		t.Fatalf("unexpected setting: %#v", setting)
	}
}

func TestSelectRejectsChangingImmutableWebServer(t *testing.T) {
	store := newMemoryStore()
	store.settings[WebServerSettingKey] = Setting{
		Key:         WebServerSettingKey,
		Value:       "nginx",
		IsImmutable: true,
	}
	service := NewWebServerService(store)

	_, err := service.Select(context.Background(), "apache")
	if !errors.Is(err, ErrWebServerAlreadySelected) {
		t.Fatalf("expected ErrWebServerAlreadySelected, got %v", err)
	}
}

func TestSelectRecordsSelectedAuditEvent(t *testing.T) {
	store := newMemoryStore()
	events := &recordingEvents{}
	service := NewWebServerService(store).WithAudit(audit.NewWriter(events))

	if _, err := service.Select(context.Background(), "nginx"); err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if len(events.events) != 1 {
		t.Fatalf("recorded events = %d, want 1", len(events.events))
	}
	event := events.events[0]
	if event.Action != audit.ActionWebServerSelected || event.TargetType != "server_setting" || event.TargetID != WebServerSettingKey {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.Metadata["value"] != "nginx" {
		t.Fatalf("unexpected metadata: %#v", event.Metadata)
	}
}

func TestSelectRecordsChangeDeniedAuditEvent(t *testing.T) {
	store := newMemoryStore()
	store.settings[WebServerSettingKey] = Setting{
		Key:         WebServerSettingKey,
		Value:       "nginx",
		IsImmutable: true,
	}
	events := &recordingEvents{}
	service := NewWebServerService(store).WithAudit(audit.NewWriter(events))

	_, err := service.Select(context.Background(), "apache")
	if !errors.Is(err, ErrWebServerAlreadySelected) {
		t.Fatalf("expected ErrWebServerAlreadySelected, got %v", err)
	}
	if len(events.events) != 1 {
		t.Fatalf("recorded events = %d, want 1", len(events.events))
	}
	event := events.events[0]
	if event.Action != audit.ActionWebServerChangeDenied || event.TargetType != "server_setting" || event.TargetID != WebServerSettingKey {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.Metadata["value"] != "apache" {
		t.Fatalf("unexpected metadata: %#v", event.Metadata)
	}
}

func TestSelectedRequiresExistingValue(t *testing.T) {
	service := NewWebServerService(newMemoryStore())

	_, err := service.Selected(context.Background())
	if !errors.Is(err, ErrSettingNotFound) {
		t.Fatalf("expected ErrSettingNotFound, got %v", err)
	}
}

func TestSelectedRejectsEmptyValue(t *testing.T) {
	store := newMemoryStore()
	store.settings[WebServerSettingKey] = Setting{Key: WebServerSettingKey, IsImmutable: true}
	service := NewWebServerService(store)

	_, err := service.Selected(context.Background())
	if !errors.Is(err, ErrWebServerNotSelected) {
		t.Fatalf("expected ErrWebServerNotSelected, got %v", err)
	}
}

type memoryStore struct {
	settings map[string]Setting
}

func newMemoryStore() *memoryStore {
	return &memoryStore{settings: make(map[string]Setting)}
}

func (s *memoryStore) Get(_ context.Context, key string) (Setting, error) {
	setting, ok := s.settings[key]
	if !ok {
		return Setting{}, ErrSettingNotFound
	}
	return setting, nil
}

func (s *memoryStore) Save(_ context.Context, setting Setting) error {
	s.settings[setting.Key] = setting
	return nil
}

type recordingEvents struct {
	events []audit.Event
}

func (r *recordingEvents) Write(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}
