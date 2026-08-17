package installer

import (
	"context"
	"errors"
	"testing"

	"github.com/motekar/motekar-panel/internal/settings"
)

func TestWebServerExecutorSupportsOnlyWebServerSettingAction(t *testing.T) {
	service := settings.NewWebServerService(newApplyMemoryStore())
	executor := WebServerExecutor{Service: service, Value: "nginx"}

	for _, id := range []string{"postgresql.install", "webserver.install", "database.migrate", "systemd.services"} {
		if err := executor.Execute(context.Background(), Action{ID: id, ChangesHost: true}); !errors.Is(err, ErrUnsupportedAction) {
			t.Fatalf("Execute(%q) error = %v, want %v", id, err, ErrUnsupportedAction)
		}
	}
}

func TestWebServerExecutorSelectsWebServer(t *testing.T) {
	store := newApplyMemoryStore()
	service := settings.NewWebServerService(store)
	executor := WebServerExecutor{Service: service, Value: "nginx"}

	if err := executor.Execute(context.Background(), Action{ID: "settings.webserver", ChangesHost: true}); err != nil {
		t.Fatalf("Execute(settings.webserver): %v", err)
	}
	selected, err := service.Selected(context.Background())
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}
	if selected != settings.WebServerNginx {
		t.Fatalf("selected = %q, want nginx", selected)
	}
}

func TestWebServerExecutorRejectsChangeOfImmutableWebServer(t *testing.T) {
	store := newApplyMemoryStore()
	service := settings.NewWebServerService(store)
	if _, err := service.Select(context.Background(), "nginx"); err != nil {
		t.Fatalf("first Select: %v", err)
	}

	executor := WebServerExecutor{Service: service, Value: "apache"}
	err := executor.Execute(context.Background(), Action{ID: "settings.webserver", ChangesHost: true})
	if !errors.Is(err, settings.ErrWebServerAlreadySelected) {
		t.Fatalf("Execute error = %v, want %v", err, settings.ErrWebServerAlreadySelected)
	}
}

type applyMemoryStore struct {
	settings map[string]settings.Setting
}

func newApplyMemoryStore() *applyMemoryStore {
	return &applyMemoryStore{settings: make(map[string]settings.Setting)}
}

func (s *applyMemoryStore) Get(_ context.Context, key string) (settings.Setting, error) {
	setting, ok := s.settings[key]
	if !ok {
		return settings.Setting{}, settings.ErrSettingNotFound
	}
	return setting, nil
}

func (s *applyMemoryStore) Save(_ context.Context, setting settings.Setting) error {
	if existing, ok := s.settings[setting.Key]; ok && existing.Value != "" && existing.IsImmutable {
		return settings.ErrWebServerAlreadySelected
	}
	s.settings[setting.Key] = setting
	return nil
}
