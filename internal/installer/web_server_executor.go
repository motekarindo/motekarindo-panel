package installer

import (
	"context"
	"fmt"

	"github.com/motekar/motekar-panel/internal/settings"
)

type WebServerSelector interface {
	Select(ctx context.Context, value string) (settings.WebServer, error)
}

type WebServerExecutor struct {
	Service WebServerSelector
	Value   string
}

func (e WebServerExecutor) Execute(ctx context.Context, action Action) error {
	if action.ID != "settings.webserver" {
		return fmt.Errorf("%w: %s", ErrUnsupportedAction, action.ID)
	}
	_, err := e.Service.Select(ctx, e.Value)
	return err
}
