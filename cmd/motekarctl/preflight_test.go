package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/motekar/motekar-panel/internal/osdetect"
	"github.com/motekar/motekar-panel/internal/preflight"
)

func TestRunPreflightSampleSharedHosting(t *testing.T) {
	var stdout bytes.Buffer
	err := runPreflight([]string{"sample"}, &stdout, nil)
	if err != nil {
		t.Fatalf("runPreflight returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "PASS\tmemory\tminimum memory is 2048 MB RAM") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestRunPreflightSingleUserSampleWarnsOnOneGB(t *testing.T) {
	var stdout bytes.Buffer
	err := runPreflight([]string{"--sample", "--profile", "single-user"}, &stdout, nil)
	if err != nil {
		t.Fatalf("runPreflight returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "WARN\tmemory") {
		t.Fatalf("expected memory warning, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "PASS\tdisk\tminimum disk is 15 GB disk") {
		t.Fatalf("expected single-user disk minimum, got %q", stdout.String())
	}
}

func TestRunPreflightUsesCollector(t *testing.T) {
	var stdout bytes.Buffer
	err := runPreflight(
		[]string{"--profile", "shared-hosting", "--postgresql", "external"},
		&stdout,
		func(_ context.Context, options preflight.CollectorOptions) (preflight.SystemFacts, error) {
			if options.Profile != preflight.ProfileSharedHosting || options.PostgreSQLPlan != preflight.PostgreSQLPlanExternal {
				t.Fatalf("unexpected collector options: %#v", options)
			}
			return preflight.SystemFacts{
				OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "24.04"},
				Profile:        options.Profile,
				CPUCores:       1,
				RAMMB:          2048,
				DiskGB:         20,
				SwapMB:         1024,
				IsRoot:         true,
				HasSystemd:     true,
				PortsAvailable: map[int]bool{80: true, 443: true},
				PostgreSQLPlan: options.PostgreSQLPlan,
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("runPreflight returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "PASS\tpostgresql\tPostgreSQL plan: external") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestRunPreflightFailsOnBlockingFailures(t *testing.T) {
	var stdout bytes.Buffer
	err := runPreflight(
		[]string{"--sample", "--postgresql", "external"},
		&stdout,
		func(context.Context, preflight.CollectorOptions) (preflight.SystemFacts, error) {
			t.Fatal("collector should not be called for sample")
			return preflight.SystemFacts{}, nil
		},
	)
	if err != nil {
		t.Fatalf("sample should pass, got %v", err)
	}

	stdout.Reset()
	err = runPreflight(
		[]string{"--profile", "shared-hosting"},
		&stdout,
		func(context.Context, preflight.CollectorOptions) (preflight.SystemFacts, error) {
			return preflight.SystemFacts{
				OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "24.04"},
				Profile:        preflight.ProfileSharedHosting,
				CPUCores:       1,
				RAMMB:          1024,
				DiskGB:         20,
				SwapMB:         1024,
				IsRoot:         true,
				HasSystemd:     true,
				PortsAvailable: map[int]bool{80: true, 443: true},
				PostgreSQLPlan: preflight.PostgreSQLPlanInstall,
			}, nil
		},
	)
	if err == nil {
		t.Fatal("expected blocking failure")
	}
}
