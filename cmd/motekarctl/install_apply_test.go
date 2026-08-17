package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/motekar/motekar-panel/internal/installer"
	"github.com/motekar/motekar-panel/internal/osdetect"
	"github.com/motekar/motekar-panel/internal/preflight"
)

func TestRunInstallApplySamplePersistsWebServer(t *testing.T) {
	var stdout bytes.Buffer
	var called bool
	err := runInstallApply(
		[]string{"--sample", "--profile", "shared-hosting", "--web-server", "nginx", "--postgresql", "install", "--database-url", "postgres://example/test"},
		&stdout,
		nil,
		func(_ context.Context, url, value string) (installer.ActionExecutor, io.Closer, error) {
			called = true
			if url != "postgres://example/test" {
				t.Fatalf("database URL = %q", url)
			}
			if value != "nginx" {
				t.Fatalf("web server value = %q", value)
			}
			return applyRecordingExecutor{}, nil, nil
		},
	)
	if err != nil {
		t.Fatalf("runInstallApply returned error: %v", err)
	}
	if !called {
		t.Fatal("openExecutor was not called")
	}
	if !strings.Contains(stdout.String(), "web_server: nginx") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestOpenInstallExecutorRequiresDatabaseURL(t *testing.T) {
	t.Setenv("MOTEKAR_DATABASE_URL", "")
	_, _, err := openInstallExecutor(context.Background(), "", "nginx")
	if err == nil {
		t.Fatal("expected database URL error")
	}
	if !strings.Contains(err.Error(), "database URL is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInstallApplyUsesCollector(t *testing.T) {
	var stdout bytes.Buffer
	err := runInstallApply(
		[]string{"--profile", "single-user", "--web-server", "nginx", "--postgresql", "external", "--database-url", "postgres://example/test"},
		&stdout,
		func(_ context.Context, options preflight.CollectorOptions) (preflight.SystemFacts, error) {
			if options.Profile != preflight.ProfileSingleUser || options.PostgreSQLPlan != preflight.PostgreSQLPlanExternal {
				t.Fatalf("unexpected collector options: %#v", options)
			}
			return preflight.SystemFacts{
				OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "24.04"},
				Profile:        options.Profile,
				CPUCores:       1,
				RAMMB:          961,
				DiskGB:         15,
				SwapMB:         1024,
				IsRoot:         true,
				HasSystemd:     true,
				PortsAvailable: map[int]bool{80: true, 443: true},
				PostgreSQLPlan: options.PostgreSQLPlan,
			}, nil
		},
		func(_ context.Context, _, _ string) (installer.ActionExecutor, io.Closer, error) {
			return applyRecordingExecutor{}, nil, nil
		},
	)
	if err != nil {
		t.Fatalf("runInstallApply returned error: %v", err)
	}
}

func TestRunInstallApplyReportsSkippedActions(t *testing.T) {
	var stdout bytes.Buffer
	err := runInstallApply(
		[]string{"--sample", "--profile", "shared-hosting", "--web-server", "nginx", "--postgresql", "install", "--database-url", "postgres://example/test"},
		&stdout,
		nil,
		func(_ context.Context, _, _ string) (installer.ActionExecutor, io.Closer, error) {
			return applyRecordingExecutor{err: installer.ErrUnsupportedAction}, nil, nil
		},
	)
	if err != nil {
		t.Fatalf("runInstallApply returned error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "skipped") || !strings.Contains(output, "web_server: nginx") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestRunInstallApplyFailsOnRealExecutorError(t *testing.T) {
	var stdout bytes.Buffer
	wantErr := errors.New("executor failure")
	err := runInstallApply(
		[]string{"--sample", "--profile", "shared-hosting", "--web-server", "nginx", "--postgresql", "install", "--database-url", "postgres://example/test"},
		&stdout,
		nil,
		func(_ context.Context, _, _ string) (installer.ActionExecutor, io.Closer, error) {
			return applyRecordingExecutor{err: wantErr}, nil, nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestInstallCommandRejectsUnknownSubcommand(t *testing.T) {
	if err := installCommand([]string{"bogus"}); err == nil {
		t.Fatal("expected unknown subcommand error")
	}
}

type applyRecordingExecutor struct {
	err error
}

func (e applyRecordingExecutor) Execute(_ context.Context, _ installer.Action) error {
	return e.err
}
