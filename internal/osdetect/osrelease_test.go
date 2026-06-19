package osdetect

import (
	"strings"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	release, err := ParseOSRelease(strings.NewReader(`
ID=ubuntu
VERSION_ID="24.04"
PRETTY_NAME="Ubuntu 24.04.2 LTS"
`))
	if err != nil {
		t.Fatalf("parse os-release: %v", err)
	}

	if release.ID != "ubuntu" {
		t.Fatalf("ID = %q, want ubuntu", release.ID)
	}
	if release.VersionID != "24.04" {
		t.Fatalf("VersionID = %q, want 24.04", release.VersionID)
	}
	if release.Name != "Ubuntu 24.04.2 LTS" {
		t.Fatalf("Name = %q", release.Name)
	}
}

func TestCheckSupportAllowsUbuntu2404(t *testing.T) {
	status := CheckSupport(OSRelease{ID: "ubuntu", VersionID: "24.04"})
	if !status.Supported {
		t.Fatalf("Supported = false, reason = %q", status.Reason)
	}
}

func TestCheckSupportRejectsUbuntu2204(t *testing.T) {
	status := CheckSupport(OSRelease{ID: "ubuntu", VersionID: "22.04"})
	if status.Supported {
		t.Fatal("Ubuntu 22.04 should not be supported by first installer release")
	}
}

func TestCheckSupportMentionsPlannedDebian(t *testing.T) {
	status := CheckSupport(OSRelease{ID: "debian", VersionID: "12"})
	if status.Supported {
		t.Fatal("Debian should not be supported by first installer release")
	}
	if !strings.Contains(status.Reason, "planned") {
		t.Fatalf("Reason = %q, want planned support message", status.Reason)
	}
}

func TestCheckSupportMentionsPlannedRHELFamily(t *testing.T) {
	status := CheckSupport(OSRelease{ID: "rocky", VersionID: "9"})
	if status.Supported {
		t.Fatal("Rocky should not be supported by first installer release")
	}
	if !strings.Contains(status.Reason, "RHEL-compatible") {
		t.Fatalf("Reason = %q, want RHEL-compatible support message", status.Reason)
	}
}
