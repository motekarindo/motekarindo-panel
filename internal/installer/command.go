package installer

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
)

// CommandRunner runs host commands during actual install. It is abstracted so
// unit tests can record or stub commands without touching the host.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execCommandRunner runs host commands, streaming their output to a writer as
// it arrives so long-running installs (apt-get, systemctl) show live progress
// and keep the SSH session from looking idle. Captured output is still
// returned so callers can include it in error messages.
type execCommandRunner struct {
	writer io.Writer
}

func newExecCommandRunner() execCommandRunner {
	return execCommandRunner{writer: os.Stderr}
}

func (r execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

	var buf bytes.Buffer
	writer := io.MultiWriter(r.writer, &buf)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return buf.Bytes(), err
	}
	return buf.Bytes(), nil
}
