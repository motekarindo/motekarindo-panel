package preflight

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/motekar/motekar-panel/internal/osdetect"
)

type CollectorOptions struct {
	Profile        InstallProfile
	PostgreSQLPlan PostgreSQLPlan
	DiskPath       string
	Ports          []int
}

type Collector struct {
	OSReleasePath string
	MeminfoPath   string
	SystemdPath   string
	Proc1CommPath string
	DiskPath      string
	DialTimeout   time.Duration
}

func NewCollector() Collector {
	return Collector{
		OSReleasePath: "/etc/os-release",
		MeminfoPath:   "/proc/meminfo",
		SystemdPath:   "/run/systemd/system",
		Proc1CommPath: "/proc/1/comm",
		DiskPath:      "/",
		DialTimeout:   500 * time.Millisecond,
	}
}

func (c Collector) Collect(ctx context.Context, options CollectorOptions) (SystemFacts, error) {
	profile, err := ParseInstallProfile(string(options.Profile))
	if err != nil {
		return SystemFacts{}, err
	}
	postgresPlan := options.PostgreSQLPlan
	if postgresPlan == PostgreSQLPlanUnknown {
		postgresPlan = PostgreSQLPlanInstall
	}

	release, err := c.readOSRelease()
	if err != nil {
		return SystemFacts{}, err
	}
	mem, err := c.readMemory()
	if err != nil {
		return SystemFacts{}, err
	}
	diskPath := firstNonEmpty(options.DiskPath, c.DiskPath, "/")
	diskGB, err := availableDiskGB(diskPath)
	if err != nil {
		return SystemFacts{}, err
	}

	ports := options.Ports
	if ports == nil {
		ports = []int{80, 443}
	}

	return SystemFacts{
		OS:             release,
		Profile:        profile,
		CPUCores:       runtime.NumCPU(),
		RAMMB:          mem.RAMMB,
		DiskGB:         diskGB,
		SwapMB:         mem.SwapMB,
		IsRoot:         os.Geteuid() == 0,
		HasSystemd:     c.hasSystemd(),
		PortsAvailable: c.availablePorts(ctx, ports),
		PostgreSQLPlan: postgresPlan,
	}, nil
}

func (c Collector) readOSRelease() (osdetect.OSRelease, error) {
	file, err := os.Open(c.OSReleasePath)
	if err != nil {
		return osdetect.OSRelease{}, err
	}
	defer file.Close()
	return osdetect.ParseOSRelease(file)
}

type memoryInfo struct {
	RAMMB  int
	SwapMB int
}

func (c Collector) readMemory() (memoryInfo, error) {
	file, err := os.Open(c.MeminfoPath)
	if err != nil {
		return memoryInfo{}, err
	}
	defer file.Close()
	return parseMeminfo(file)
}

func parseMeminfo(file *os.File) (memoryInfo, error) {
	scanner := bufio.NewScanner(file)
	values := make(map[string]int)
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
			return memoryInfo{}, fmt.Errorf("parse %s: %w", key, err)
		}
		values[key] = kb
	}
	if err := scanner.Err(); err != nil {
		return memoryInfo{}, err
	}
	return memoryInfo{
		RAMMB:  kbToMB(values["MemTotal"]),
		SwapMB: kbToMB(values["SwapTotal"]),
	}, nil
}

func kbToMB(kb int) int {
	if kb <= 0 {
		return 0
	}
	return kb / 1024
}

func availableDiskGB(path string) (int, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	bytes := uint64(stat.Bavail) * uint64(stat.Bsize)
	return int(bytes / 1024 / 1024 / 1024), nil
}

func (c Collector) hasSystemd() bool {
	if stat, err := os.Stat(c.SystemdPath); err == nil && stat.IsDir() {
		return true
	}
	content, err := os.ReadFile(c.Proc1CommPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(content)) == "systemd"
}

func (c Collector) availablePorts(ctx context.Context, ports []int) map[int]bool {
	out := make(map[int]bool, len(ports))
	for _, port := range ports {
		out[port] = c.portAvailable(ctx, port)
	}
	return out
}

func (c Collector) portAvailable(ctx context.Context, port int) bool {
	address := fmt.Sprintf(":%d", port)
	timeout := c.DialTimeout
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", address)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
