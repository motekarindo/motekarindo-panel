package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/motekar/motekar-panel/internal/audit"
)

const WebServerSettingKey = "web_server"

type WebServer string

const (
	WebServerNginx  WebServer = "nginx"
	WebServerApache WebServer = "apache"
)

var (
	ErrSettingNotFound          = errors.New("setting not found")
	ErrUnsupportedWebServer     = errors.New("unsupported web server")
	ErrWebServerAlreadySelected = errors.New("web server is already selected")
	ErrWebServerNotSelected     = errors.New("web server is not selected")
)

type Setting struct {
	Key         string
	Value       string
	IsImmutable bool
}

type Store interface {
	Get(ctx context.Context, key string) (Setting, error)
	Save(ctx context.Context, setting Setting) error
}

type WebServerService struct {
	store  Store
	record auditRecorder
}

type auditRecorder interface {
	Record(ctx context.Context, event audit.Event) (audit.Event, error)
}

func NewWebServerService(store Store) WebServerService {
	return WebServerService{store: store}
}

func (s WebServerService) WithAudit(record auditRecorder) WebServerService {
	s.record = record
	return s
}

func ParseWebServer(value string) (WebServer, error) {
	switch WebServer(strings.ToLower(strings.TrimSpace(value))) {
	case WebServerNginx:
		return WebServerNginx, nil
	case WebServerApache:
		return WebServerApache, nil
	default:
		return "", ErrUnsupportedWebServer
	}
}

func (s WebServerService) Select(ctx context.Context, value string) (WebServer, error) {
	selected, err := ParseWebServer(value)
	if err != nil {
		return "", err
	}

	current, err := s.store.Get(ctx, WebServerSettingKey)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return "", err
	}
	if err == nil && current.Value != "" && current.IsImmutable {
		s.recordChangeDenied(ctx, string(selected), current.Value)
		return "", fmt.Errorf("%w: %s", ErrWebServerAlreadySelected, current.Value)
	}

	if err := s.store.Save(ctx, Setting{
		Key:         WebServerSettingKey,
		Value:       string(selected),
		IsImmutable: true,
	}); err != nil {
		if errors.Is(err, ErrWebServerAlreadySelected) {
			s.recordChangeDenied(ctx, string(selected), "")
		}
		return "", err
	}

	s.recordSelected(ctx, string(selected))
	return selected, nil
}

func (s WebServerService) recordSelected(ctx context.Context, value string) {
	if s.record == nil {
		return
	}
	_, _ = s.record.Record(ctx, audit.Event{
		Action:     audit.ActionWebServerSelected,
		TargetType: "server_setting",
		TargetID:   WebServerSettingKey,
		Metadata:   map[string]string{"value": value},
	})
}

func (s WebServerService) recordChangeDenied(ctx context.Context, requested, current string) {
	if s.record == nil {
		return
	}
	metadata := map[string]string{"value": requested}
	if current != "" {
		metadata["current"] = current
	}
	_, _ = s.record.Record(ctx, audit.Event{
		Action:     audit.ActionWebServerChangeDenied,
		TargetType: "server_setting",
		TargetID:   WebServerSettingKey,
		Metadata:   metadata,
	})
}

func (s WebServerService) Selected(ctx context.Context) (WebServer, error) {
	setting, err := s.store.Get(ctx, WebServerSettingKey)
	if err != nil {
		return "", err
	}
	if setting.Value == "" {
		return "", ErrWebServerNotSelected
	}
	return ParseWebServer(setting.Value)
}
