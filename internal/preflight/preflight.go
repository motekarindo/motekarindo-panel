package preflight

import (
	"fmt"
	"sort"

	"github.com/motekar/motekar-panel/internal/osdetect"
)

const (
	MinimumCPUCores = 1
	MinimumRAMMB    = 2048
	MinimumDiskGB   = 20
	MinimumSwapMB   = 1024
)

type PostgreSQLPlan string

const (
	PostgreSQLPlanExternal PostgreSQLPlan = "external"
	PostgreSQLPlanInstall  PostgreSQLPlan = "install"
	PostgreSQLPlanUnknown  PostgreSQLPlan = ""
)

type SystemFacts struct {
	OS             osdetect.OSRelease
	CPUCores       int
	RAMMB          int
	DiskGB         int
	SwapMB         int
	IsRoot         bool
	HasSystemd     bool
	PortsAvailable map[int]bool
	PostgreSQLPlan PostgreSQLPlan
}

type CheckStatus string

const (
	CheckPass CheckStatus = "PASS"
	CheckFail CheckStatus = "FAIL"
	CheckWarn CheckStatus = "WARN"
)

type Check struct {
	Name     string
	Status   CheckStatus
	Message  string
	Blocking bool
}

type Report struct {
	Checks []Check
}

func (r Report) BlockingFailures() []Check {
	var failures []Check
	for _, check := range r.Checks {
		if check.Blocking && check.Status == CheckFail {
			failures = append(failures, check)
		}
	}
	return failures
}

func (r Report) Ready() bool {
	return len(r.BlockingFailures()) == 0
}

func Run(facts SystemFacts) Report {
	checks := []Check{
		checkOS(facts.OS),
		minimum("cpu", facts.CPUCores, MinimumCPUCores, "core(s)"),
		minimum("memory", facts.RAMMB, MinimumRAMMB, "MB RAM"),
		minimum("disk", facts.DiskGB, MinimumDiskGB, "GB disk"),
		minimum("swap", facts.SwapMB, MinimumSwapMB, "MB swap"),
		boolean("root", facts.IsRoot, "installer must run as root"),
		boolean("systemd", facts.HasSystemd, "systemd is required"),
		checkPostgreSQLPlan(facts.PostgreSQLPlan),
	}

	for _, port := range sortedPorts(facts.PortsAvailable) {
		available := facts.PortsAvailable[port]
		checks = append(checks, Check{
			Name:     fmt.Sprintf("port:%d", port),
			Status:   status(available),
			Message:  fmt.Sprintf("port %d must be available", port),
			Blocking: true,
		})
	}

	return Report{Checks: checks}
}

func checkOS(release osdetect.OSRelease) Check {
	support := osdetect.CheckSupport(release)
	return Check{
		Name:     "os",
		Status:   status(support.Supported),
		Message:  support.Reason,
		Blocking: true,
	}
}

func minimum(name string, got, want int, unit string) Check {
	ok := got >= want
	return Check{
		Name:     name,
		Status:   status(ok),
		Message:  fmt.Sprintf("minimum %s is %d %s; detected %d", name, want, unit, got),
		Blocking: true,
	}
}

func boolean(name string, ok bool, message string) Check {
	return Check{
		Name:     name,
		Status:   status(ok),
		Message:  message,
		Blocking: true,
	}
}

func checkPostgreSQLPlan(plan PostgreSQLPlan) Check {
	switch plan {
	case PostgreSQLPlanExternal, PostgreSQLPlanInstall:
		return Check{
			Name:     "postgresql",
			Status:   CheckPass,
			Message:  fmt.Sprintf("PostgreSQL plan: %s", plan),
			Blocking: true,
		}
	default:
		return Check{
			Name:     "postgresql",
			Status:   CheckFail,
			Message:  "PostgreSQL must be installed by Motekar Panel or provided externally",
			Blocking: true,
		}
	}
}

func status(ok bool) CheckStatus {
	if ok {
		return CheckPass
	}
	return CheckFail
}

func sortedPorts(ports map[int]bool) []int {
	out := make([]int, 0, len(ports))
	for port := range ports {
		out = append(out, port)
	}
	sort.Ints(out)
	return out
}
