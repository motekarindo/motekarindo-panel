package installer

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestExecCommandRunnerStreamsOutputAndCaptures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix echo")
	}

	var streamed bytes.Buffer
	runner := execCommandRunner{writer: &streamed}

	output, err := runner.Run(context.Background(), "echo", "hello-stream")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(string(output), "hello-stream") {
		t.Fatalf("captured output = %q", output)
	}
	if !strings.Contains(streamed.String(), "hello-stream") {
		t.Fatalf("streamed output = %q", streamed.String())
	}
}

func TestExecCommandRunnerSetsNoninteractiveFrontend(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix sh")
	}

	var streamed bytes.Buffer
	runner := execCommandRunner{writer: &streamed}

	_, err := runner.Run(context.Background(), "sh", "-c", "printf '%s' \"$DEBIAN_FRONTEND\"")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if streamed.String() != "noninteractive" {
		t.Fatalf("DEBIAN_FRONTEND = %q", streamed.String())
	}
}

func TestExecCommandRunnerReturnsOutputOnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix sh")
	}

	var streamed bytes.Buffer
	runner := execCommandRunner{writer: &streamed}

	output, err := runner.Run(context.Background(), "sh", "-c", "echo fail-output >&2; exit 7")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(string(output), "fail-output") {
		t.Fatalf("captured output on error = %q", output)
	}
}
