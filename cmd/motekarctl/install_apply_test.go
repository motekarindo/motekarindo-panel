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
		[]string{"--sample", "--profile", "shared-hosting", "--web-server", "nginx", "--postgresql", "install"},
		&stdout,
		nil,
		func(_ context.Context, options installApplyOptions) (installer.ActionExecutor, io.Closer, error) {
			called = true
			if options.webServer != "nginx" {
				t.Fatalf("web server value = %q", options.webServer)
			}
			if options.databaseURL != "" {
				t.Fatalf("database URL = %q", options.databaseURL)
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

func TestRunInstallApplyUsesCollector(t *testing.T) {
	var stdout bytes.Buffer
	err := runInstallApply(
		[]string{"--profile", "single-user", "--web-server", "nginx", "--postgresql", "external"},
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
		func(_ context.Context, _ installApplyOptions) (installer.ActionExecutor, io.Closer, error) {
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
		[]string{"--sample", "--profile", "shared-hosting", "--web-server", "nginx", "--postgresql", "install"},
		&stdout,
		nil,
		func(_ context.Context, _ installApplyOptions) (installer.ActionExecutor, io.Closer, error) {
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
		[]string{"--sample", "--profile", "shared-hosting", "--web-server", "nginx", "--postgresql", "install"},
		&stdout,
		nil,
		func(_ context.Context, _ installApplyOptions) (installer.ActionExecutor, io.Closer, error) {
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

func TestParseInstallApplyOptionsReadsAdminPasswordFromStdin(t *testing.T) {
	options, err := parseInstallApplyOptionsFromReader(
		[]string{"--sample", "--web-server", "nginx", "--admin-email", "owner@example.com", "--admin-display-name", "Owner", "--admin-password-stdin"},
		strings.NewReader("hunter2secret\n"),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if options.adminPassword != "hunter2secret" {
		t.Fatalf("adminPassword = %q", options.adminPassword)
	}
}

func TestParseInstallApplyOptionsRejectsEmptyAdminPassword(t *testing.T) {
	_, err := parseInstallApplyOptionsFromReader(
		[]string{"--sample", "--web-server", "nginx", "--admin-email", "owner@example.com", "--admin-password-stdin"},
		strings.NewReader("\n"),
	)
	if err == nil {
		t.Fatal("expected empty password error")
	}
}

func TestParseInstallApplyOptionsRejectsUnknownPostgresqlPlan(t *testing.T) {
	_, err := parseInstallApplyOptionsFromReader(
		[]string{"--sample", "--web-server", "nginx", "--postgresql", "bogus"},
		strings.NewReader(""),
	)
	if err == nil {
		t.Fatal("expected unsupported PostgreSQL plan error")
	}
}

type applyRecordingExecutor struct {
	err error
}

func (e applyRecordingExecutor) Execute(_ context.Context, _ installer.Action) error {
	return e.err
}
