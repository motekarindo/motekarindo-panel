package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryCollectorGathersSystemFacts(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "os-release"), "ID=ubuntu\nVERSION_ID=\"24.04\"\nNAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n")
	writeTestFile(t, filepath.Join(dir, "osrelease"), "6.8.0-45-generic\n")
	writeTestFile(t, filepath.Join(dir, "meminfo"), "MemTotal:       16000000 kB\nMemAvailable:    8000000 kB\nSwapTotal:       2000000 kB\n")
	writeTestFile(t, filepath.Join(dir, "loadavg"), "0.52 0.35 0.25 1/100 1234\n")
	writeTestFile(t, filepath.Join(dir, "uptime"), "4321.50 12345.67\n")
	writeTestFile(t, filepath.Join(dir, "proc1comm"), "systemd\n")
	if err := os.MkdirAll(filepath.Join(dir, "systemd"), 0o755); err != nil {
		t.Fatalf("create systemd dir: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "systemd", "nginx.service"), "# placeholder\n")
	writeTestFile(t, filepath.Join(dir, "systemd", "postgresql.service"), "# placeholder\n")
	writeTestFile(t, filepath.Join(dir, "systemd", "random.conf"), "# placeholder\n")

	collector := InventoryCollector{
		OSReleasePath: filepath.Join(dir, "os-release"),
		KernelPath:    filepath.Join(dir, "osrelease"),
		MeminfoPath:   filepath.Join(dir, "meminfo"),
		LoadavgPath:   filepath.Join(dir, "loadavg"),
		UptimePath:    filepath.Join(dir, "uptime"),
		SystemdPath:   filepath.Join(dir, "systemd"),
		Proc1CommPath: filepath.Join(dir, "proc1comm"),
		DiskPath:      dir,
		IPAddresses: func() ([]string, error) {
			return []string{"192.0.2.10", "2001:db8::10"}, nil
		},
	}

	inventory := collector.Collect()
	if inventory.OS.ID != "ubuntu" || inventory.OS.VersionID != "24.04" || inventory.OS.Name != "Ubuntu 24.04 LTS" {
		t.Fatalf("OS = %#v", inventory.OS)
	}
	if inventory.Kernel != "6.8.0-45-generic" {
		t.Fatalf("kernel = %q", inventory.Kernel)
	}
	if inventory.RAMTotalMB != 15625 || inventory.RAMAvailableMB != 7812 || inventory.SwapMB != 1953 {
		t.Fatalf("memory = total:%d avail:%d swap:%d", inventory.RAMTotalMB, inventory.RAMAvailableMB, inventory.SwapMB)
	}
	if inventory.Load1 != 0.52 || inventory.Load5 != 0.35 || inventory.Load15 != 0.25 {
		t.Fatalf("load = %f %f %f", inventory.Load1, inventory.Load5, inventory.Load15)
	}
	if inventory.UptimeSeconds != 4321 {
		t.Fatalf("uptime = %d", inventory.UptimeSeconds)
	}
	if len(inventory.IPAddresses) != 2 || inventory.IPAddresses[0] != "192.0.2.10" {
		t.Fatalf("IPs = %#v", inventory.IPAddresses)
	}
	if !inventory.HasSystemd {
		t.Fatal("expected systemd detected")
	}
	if len(inventory.Services) != 2 || inventory.Services[0] != "nginx" || inventory.Services[1] != "postgresql" {
		t.Fatalf("services = %#v", inventory.Services)
	}
}

func TestInventoryCollectorDetectsSystemdFromProc1(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "os-release"), "ID=debian\nVERSION_ID=\"12\"\nNAME=\"Debian\"\n")
	writeTestFile(t, filepath.Join(dir, "osrelease"), "6.1.0-18-amd64\n")
	writeTestFile(t, filepath.Join(dir, "meminfo"), "MemTotal:        8000000 kB\nMemAvailable:     4000000 kB\nSwapTotal:       0 kB\n")
	writeTestFile(t, filepath.Join(dir, "loadavg"), "0.10 0.20 0.30 1/100 123\n")
	writeTestFile(t, filepath.Join(dir, "uptime"), "100.00 50.00\n")
	writeTestFile(t, filepath.Join(dir, "proc1comm"), "systemd\n")

	collector := InventoryCollector{
		OSReleasePath: filepath.Join(dir, "os-release"),
		KernelPath:    filepath.Join(dir, "osrelease"),
		MeminfoPath:   filepath.Join(dir, "meminfo"),
		LoadavgPath:   filepath.Join(dir, "loadavg"),
		UptimePath:    filepath.Join(dir, "uptime"),
		SystemdPath:   filepath.Join(dir, "missing-systemd"),
		Proc1CommPath: filepath.Join(dir, "proc1comm"),
		DiskPath:      dir,
		IPAddresses:   func() ([]string, error) { return nil, nil },
	}

	inventory := collector.Collect()
	if !inventory.HasSystemd {
		t.Fatal("expected systemd detected from /proc/1/comm")
	}
}

func TestInventoryCollectorToleratesMissingSources(t *testing.T) {
	dir := t.TempDir()
	collector := InventoryCollector{
		OSReleasePath: filepath.Join(dir, "missing-os-release"),
		KernelPath:    filepath.Join(dir, "missing-kernel"),
		MeminfoPath:   filepath.Join(dir, "missing-meminfo"),
		LoadavgPath:   filepath.Join(dir, "missing-loadavg"),
		UptimePath:    filepath.Join(dir, "missing-uptime"),
		SystemdPath:   filepath.Join(dir, "missing-systemd"),
		Proc1CommPath: filepath.Join(dir, "missing-proc1"),
		DiskPath:      filepath.Join(dir, "missing-disk"),
		IPAddresses:   func() ([]string, error) { return nil, errors.New("no interfaces") },
	}

	inventory := collector.Collect()
	if inventory.OS.ID != "" || inventory.Kernel != "" {
		t.Fatalf("missing sources should yield empty values: %#v", inventory)
	}
	if inventory.RAMTotalMB != 0 || inventory.RAMAvailableMB != 0 || inventory.SwapMB != 0 {
		t.Fatalf("missing meminfo should yield zero memory: %#v", inventory)
	}
	if inventory.DiskFreeGB != 0 {
		t.Fatalf("missing disk should yield zero: %#v", inventory)
	}
	if inventory.Load1 != 0 || inventory.Load5 != 0 || inventory.Load15 != 0 {
		t.Fatalf("missing loadavg should yield zero load: %#v", inventory)
	}
	if inventory.UptimeSeconds != 0 || inventory.HasSystemd || len(inventory.Services) != 0 || len(inventory.IPAddresses) != 0 {
		t.Fatalf("missing sources should yield zero values: %#v", inventory)
	}
}

func TestDefaultRegistryExecutesServerInventory(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "os-release"), "ID=ubuntu\nVERSION_ID=\"24.04\"\nNAME=\"Ubuntu\"\n")
	writeTestFile(t, filepath.Join(dir, "osrelease"), "6.8.0-45-generic\n")
	writeTestFile(t, filepath.Join(dir, "meminfo"), "MemTotal:        8000000 kB\nMemAvailable:     4000000 kB\nSwapTotal:       0 kB\n")
	writeTestFile(t, filepath.Join(dir, "loadavg"), "0.10 0.20 0.30 1/100 123\n")
	writeTestFile(t, filepath.Join(dir, "uptime"), "100.00 50.00\n")

	collector := InventoryCollector{
		OSReleasePath: filepath.Join(dir, "os-release"),
		KernelPath:    filepath.Join(dir, "osrelease"),
		MeminfoPath:   filepath.Join(dir, "meminfo"),
		LoadavgPath:   filepath.Join(dir, "loadavg"),
		UptimePath:    filepath.Join(dir, "uptime"),
		SystemdPath:   filepath.Join(dir, "no-systemd"),
		Proc1CommPath: filepath.Join(dir, "no-proc1"),
		DiskPath:      dir,
		IPAddresses:   func() ([]string, error) { return nil, nil },
	}

	result, err := inventoryAction(collector)(context.Background(), EmptyPayload{})
	if err != nil {
		t.Fatalf("inventory action: %v", err)
	}
	if result.Status != "ok" || result.Action != "server.inventory" {
		t.Fatalf("result = %#v", result)
	}
	if result.Data["kernel"] != "6.8.0-45-generic" || result.Data["cpuCores"] == 0 {
		t.Fatalf("result data = %#v", result.Data)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
