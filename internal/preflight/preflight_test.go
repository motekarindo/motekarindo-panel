package preflight

import (
	"testing"

	"github.com/motekar/motekar-panel/internal/osdetect"
)

func TestRunPassesMinimumSupportedFacts(t *testing.T) {
	report := Run(SystemFacts{
		OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "24.04"},
		CPUCores:       1,
		RAMMB:          2048,
		DiskGB:         20,
		SwapMB:         1024,
		IsRoot:         true,
		HasSystemd:     true,
		PortsAvailable: map[int]bool{80: true, 443: true},
		PostgreSQLPlan: PostgreSQLPlanInstall,
	})

	if !report.Ready() {
		t.Fatalf("report should be ready, blocking failures = %#v", report.BlockingFailures())
	}
}

func TestRunBlocksUnsupportedOS(t *testing.T) {
	report := Run(SystemFacts{
		OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "22.04"},
		CPUCores:       1,
		RAMMB:          2048,
		DiskGB:         20,
		SwapMB:         1024,
		IsRoot:         true,
		HasSystemd:     true,
		PortsAvailable: map[int]bool{80: true, 443: true},
		PostgreSQLPlan: PostgreSQLPlanInstall,
	})

	if report.Ready() {
		t.Fatal("report should not be ready for unsupported OS")
	}
	if !hasFailure(report, "os") {
		t.Fatalf("expected os failure, got %#v", report.Checks)
	}
}

func TestRunBlocksLowResources(t *testing.T) {
	report := Run(SystemFacts{
		OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "24.04"},
		CPUCores:       0,
		RAMMB:          1024,
		DiskGB:         10,
		SwapMB:         0,
		IsRoot:         true,
		HasSystemd:     true,
		PortsAvailable: map[int]bool{80: true, 443: true},
		PostgreSQLPlan: PostgreSQLPlanInstall,
	})

	for _, name := range []string{"cpu", "memory", "disk", "swap"} {
		if !hasFailure(report, name) {
			t.Fatalf("expected %s failure, got %#v", name, report.Checks)
		}
	}
}

func TestRunBlocksUnavailablePortsAndMissingPostgreSQLPlan(t *testing.T) {
	report := Run(SystemFacts{
		OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "24.04"},
		CPUCores:       1,
		RAMMB:          2048,
		DiskGB:         20,
		SwapMB:         1024,
		IsRoot:         true,
		HasSystemd:     true,
		PortsAvailable: map[int]bool{80: false, 443: true},
		PostgreSQLPlan: PostgreSQLPlanUnknown,
	})

	if !hasFailure(report, "port:80") {
		t.Fatalf("expected port:80 failure, got %#v", report.Checks)
	}
	if !hasFailure(report, "postgresql") {
		t.Fatalf("expected postgresql failure, got %#v", report.Checks)
	}
}

func hasFailure(report Report, name string) bool {
	for _, check := range report.Checks {
		if check.Name == name && check.Status == CheckFail && check.Blocking {
			return true
		}
	}
	return false
}
