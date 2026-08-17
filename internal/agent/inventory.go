package agent

import (
	"bufio"
	"context"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/motekar/motekar-panel/internal/osdetect"
)

type SystemInventory struct {
	OS             osdetect.OSRelease `json:"os"`
	Kernel         string             `json:"kernel"`
	CPUCores       int                `json:"cpuCores"`
	RAMTotalMB     int                `json:"ramTotalMB"`
	RAMAvailableMB int                `json:"ramAvailableMB"`
	SwapMB         int                `json:"swapMB"`
	DiskFreeGB     int                `json:"diskFreeGB"`
	Load1          float64            `json:"load1"`
	Load5          float64            `json:"load5"`
	Load15         float64            `json:"load15"`
	UptimeSeconds  int64              `json:"uptimeSeconds"`
	IPAddresses    []string           `json:"ipAddresses"`
	HasSystemd     bool               `json:"hasSystemd"`
	Services       []string           `json:"services"`
}

type InventoryCollector struct {
	OSReleasePath string
	MeminfoPath   string
	KernelPath    string
	LoadavgPath   string
	UptimePath    string
	SystemdPath   string
	Proc1CommPath string
	DiskPath      string
	IPAddresses   func() ([]string, error)
}

func NewInventoryCollector() InventoryCollector {
	return InventoryCollector{
		OSReleasePath: "/etc/os-release",
		MeminfoPath:   "/proc/meminfo",
		KernelPath:    "/proc/sys/kernel/osrelease",
		LoadavgPath:   "/proc/loadavg",
		UptimePath:    "/proc/uptime",
		SystemdPath:   "/run/systemd/system",
		Proc1CommPath: "/proc/1/comm",
		DiskPath:      "/",
		IPAddresses:   collectIPAddresses,
	}
}

func (c InventoryCollector) Collect() SystemInventory {
	inventory := SystemInventory{
		CPUCores: runtime.NumCPU(),
	}

	inventory.OS = c.readOSRelease()
	inventory.Kernel = c.readKernel()
	inventory.RAMTotalMB, inventory.RAMAvailableMB, inventory.SwapMB = c.readMemory()
	inventory.DiskFreeGB = c.readDiskFree()
	inventory.Load1, inventory.Load5, inventory.Load15 = c.readLoadAverage()
	inventory.UptimeSeconds = c.readUptime()
	if c.IPAddresses != nil {
		inventory.IPAddresses = c.readIPAddresses()
	}

	inventory.HasSystemd = c.hasSystemd()
	inventory.Services = c.listServices()

	return inventory
}

func (c InventoryCollector) readOSRelease() osdetect.OSRelease {
	file, err := os.Open(c.OSReleasePath)
	if err != nil {
		return osdetect.OSRelease{}
	}
	defer file.Close()
	release, err := osdetect.ParseOSRelease(file)
	if err != nil {
		return osdetect.OSRelease{}
	}
	return release
}

func (c InventoryCollector) readKernel() string {
	content, err := os.ReadFile(c.KernelPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func (c InventoryCollector) readMemory() (totalMB, availableMB, swapMB int) {
	file, err := os.Open(c.MeminfoPath)
	if err != nil {
		return 0, 0, 0
	}
	defer file.Close()

	values := make(map[string]int)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, rest, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		kb, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		values[key] = kb
	}
	return kbToMB(values["MemTotal"]), kbToMB(values["MemAvailable"]), kbToMB(values["SwapTotal"])
}

func kbToMB(kb int) int {
	if kb <= 0 {
		return 0
	}
	return kb / 1024
}

func (c InventoryCollector) readDiskFree() int {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(c.DiskPath, &stat); err != nil {
		return 0
	}
	bytes := uint64(stat.Bavail) * uint64(stat.Bsize)
	return int(bytes / 1024 / 1024 / 1024)
}

func (c InventoryCollector) readLoadAverage() (load1, load5, load15 float64) {
	content, err := os.ReadFile(c.LoadavgPath)
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(content))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	for index, field := range fields[:3] {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return 0, 0, 0
		}
		switch index {
		case 0:
			load1 = value
		case 1:
			load5 = value
		case 2:
			load15 = value
		}
	}
	return load1, load5, load15
}

func (c InventoryCollector) readUptime() int64 {
	content, err := os.ReadFile(c.UptimePath)
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(seconds)
}

func (c InventoryCollector) readIPAddresses() []string {
	addresses, err := c.IPAddresses()
	if err != nil {
		return nil
	}
	return addresses
}

func (c InventoryCollector) hasSystemd() bool {
	if stat, err := os.Stat(c.SystemdPath); err == nil && stat.IsDir() {
		return true
	}
	content, err := os.ReadFile(c.Proc1CommPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(content)) == "systemd"
}

func (c InventoryCollector) listServices() []string {
	entries, err := os.ReadDir(c.SystemdPath)
	if err != nil {
		return nil
	}
	var services []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".service") {
			continue
		}
		services = append(services, strings.TrimSuffix(name, ".service"))
	}
	return services
}

func collectIPAddresses() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var addresses []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addrs {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			addresses = append(addresses, ip.String())
		}
	}
	return addresses, nil
}

func inventoryAction(collector InventoryCollector) TypedActionHandler[EmptyPayload] {
	return func(context.Context, EmptyPayload) (ActionResult, error) {
		inventory := collector.Collect()
		return ActionResult{
			Action: "server.inventory",
			Status: "ok",
			Data: map[string]any{
				"os": map[string]string{
					"id":        inventory.OS.ID,
					"versionId": inventory.OS.VersionID,
					"name":      inventory.OS.Name,
				},
				"kernel":         inventory.Kernel,
				"cpuCores":       inventory.CPUCores,
				"ramTotalMB":     inventory.RAMTotalMB,
				"ramAvailableMB": inventory.RAMAvailableMB,
				"swapMB":         inventory.SwapMB,
				"diskFreeGB":     inventory.DiskFreeGB,
				"load1":          inventory.Load1,
				"load5":          inventory.Load5,
				"load15":         inventory.Load15,
				"uptimeSeconds":  inventory.UptimeSeconds,
				"ipAddresses":    inventory.IPAddresses,
				"hasSystemd":     inventory.HasSystemd,
				"services":       inventory.Services,
			},
		}, nil
	}
}
