package main

import (
	"fmt"
	"os"

	"github.com/motekar/motekar-panel/internal/buildinfo"
	"github.com/motekar/motekar-panel/internal/osdetect"
	"github.com/motekar/motekar-panel/internal/preflight"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "motekarctl: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "version"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "preflight":
		return preflightCommand(args[1:])
	case "version":
		fmt.Println(buildinfo.String())
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func preflightCommand(args []string) error {
	if len(args) == 0 || args[0] != "sample" {
		return fmt.Errorf("usage: motekarctl preflight sample")
	}

	report := preflight.Run(preflight.SystemFacts{
		OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "24.04", Name: "Ubuntu 24.04 LTS"},
		CPUCores:       preflight.MinimumCPUCores,
		RAMMB:          preflight.MinimumRAMMB,
		DiskGB:         preflight.MinimumDiskGB,
		SwapMB:         preflight.MinimumSwapMB,
		IsRoot:         true,
		HasSystemd:     true,
		PortsAvailable: map[int]bool{80: true, 443: true},
		PostgreSQLPlan: preflight.PostgreSQLPlanInstall,
	})

	for _, check := range report.Checks {
		fmt.Printf("%s\t%s\t%s\n", check.Status, check.Name, check.Message)
	}
	if !report.Ready() {
		return fmt.Errorf("preflight sample has blocking failures")
	}
	return nil
}
