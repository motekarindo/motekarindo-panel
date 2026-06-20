package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/motekar/motekar-panel/internal/osdetect"
	"github.com/motekar/motekar-panel/internal/preflight"
)

func TestRunInstallPlanSample(t *testing.T) {
	var stdout bytes.Buffer
	err := runInstallPlan(
		[]string{"--sample", "--profile", "shared-hosting", "--web-server", "nginx", "--postgresql", "install"},
		&stdout,
		nil,
	)
	if err != nil {
		t.Fatalf("runInstallPlan returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"mode: dry-run",
		"web_server: nginx",
		"WOULD_CHANGE\tpostgresql.install",
		"WOULD_CHANGE\tsettings.webserver",
		"No changes were made.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}

func TestRunInstallPlanRequiresWebServer(t *testing.T) {
	var stdout bytes.Buffer
	err := runInstallPlan([]string{"--sample"}, &stdout, nil)
	if err == nil {
		t.Fatal("expected missing web server error")
	}
}

func TestRunInstallPlanUsesCollector(t *testing.T) {
	var stdout bytes.Buffer
	err := runInstallPlan(
		[]string{"--profile", "single-user", "--web-server", "apache", "--postgresql", "external"},
		&stdout,
		func(_ context.Context, options preflight.CollectorOptions) (preflight.SystemFacts, error) {
			if options.Profile != preflight.ProfileSingleUser || options.PostgreSQLPlan != preflight.PostgreSQLPlanExternal {
				t.Fatalf("unexpected collector options: %#v", options)
			}
			return preflight.SystemFacts{
				OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "24.04"},
				Profile:        options.Profile,
				CPUCores:       1,
				RAMMB:          1024,
				DiskGB:         20,
				SwapMB:         2048,
				IsRoot:         true,
				HasSystemd:     true,
				PortsAvailable: map[int]bool{80: true, 443: true},
				PostgreSQLPlan: options.PostgreSQLPlan,
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("runInstallPlan returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "READ\tpostgresql.external") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}
