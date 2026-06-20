package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	store Store
}

func NewWebServerService(store Store) WebServerService {
	return WebServerService{store: store}
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
		return "", fmt.Errorf("%w: %s", ErrWebServerAlreadySelected, current.Value)
	}

	if err := s.store.Save(ctx, Setting{
		Key:         WebServerSettingKey,
		Value:       string(selected),
		IsImmutable: true,
	}); err != nil {
		return "", err
	}

	return selected, nil
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
