package installer

import (
	"context"
	"os/exec"
)

// CommandRunner runs host commands during actual install. It is abstracted so
// unit tests can record or stub commands without touching the host.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}
