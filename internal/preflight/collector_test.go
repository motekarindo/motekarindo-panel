package preflight

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseInstallProfile(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  InstallProfile
	}{
		{input: "", want: ProfileSharedHosting},
		{input: "shared-hosting", want: ProfileSharedHosting},
		{input: "single-user", want: ProfileSingleUser},
		{input: " SINGLE-USER ", want: ProfileSingleUser},
	} {
		got, err := ParseInstallProfile(tc.input)
		if err != nil {
			t.Fatalf("ParseInstallProfile(%q) returned error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("ParseInstallProfile(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseInstallProfileRejectsUnsupportedValue(t *testing.T) {
	if _, err := ParseInstallProfile("reseller"); err == nil {
		t.Fatal("expected unsupported profile error")
	}
}

func TestParsePostgreSQLPlan(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  PostgreSQLPlan
	}{
		{input: "install", want: PostgreSQLPlanInstall},
		{input: "external", want: PostgreSQLPlanExternal},
		{input: " EXTERNAL ", want: PostgreSQLPlanExternal},
	} {
		got, err := ParsePostgreSQLPlan(tc.input)
		if err != nil {
			t.Fatalf("ParsePostgreSQLPlan(%q) returned error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("ParsePostgreSQLPlan(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCollectorReadsFixtureFacts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "os-release"), "ID=ubuntu\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n")
	writeFile(t, filepath.Join(dir, "meminfo"), "MemTotal:       1048576 kB\nSwapTotal:      2097152 kB\n")
	systemdDir := filepath.Join(dir, "systemd")
	if err := os.Mkdir(systemdDir, 0o755); err != nil {
		t.Fatal(err)
	}

	collector := Collector{
		OSReleasePath: filepath.Join(dir, "os-release"),
		MeminfoPath:   filepath.Join(dir, "meminfo"),
		SystemdPath:   systemdDir,
		Proc1CommPath: filepath.Join(dir, "missing-comm"),
		DiskPath:      dir,
	}

	facts, err := collector.Collect(t.Context(), CollectorOptions{
		Profile:        ProfileSingleUser,
		PostgreSQLPlan: PostgreSQLPlanExternal,
		DiskPath:       dir,
		Ports:          []int{},
	})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if facts.OS.ID != "ubuntu" || facts.OS.VersionID != "24.04" {
		t.Fatalf("unexpected OS facts: %#v", facts.OS)
	}
	if facts.RAMMB != 1024 || facts.SwapMB != 2048 || !facts.HasSystemd {
		t.Fatalf("unexpected collected facts: %#v", facts)
	}
	if facts.Profile != ProfileSingleUser || facts.PostgreSQLPlan != PostgreSQLPlanExternal {
		t.Fatalf("unexpected plans: %#v", facts)
	}
}

func TestParseMeminfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meminfo")
	writeFile(t, path, "MemTotal:       2097152 kB\nSwapTotal:      1048576 kB\n")

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	info, err := parseMeminfo(file)
	if err != nil {
		t.Fatalf("parseMeminfo returned error: %v", err)
	}
	if info.RAMMB != 2048 || info.SwapMB != 1024 {
		t.Fatalf("unexpected memory info: %#v", info)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
