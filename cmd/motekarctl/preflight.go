package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/motekar/motekar-panel/internal/osdetect"
	"github.com/motekar/motekar-panel/internal/preflight"
)

type preflightOptions struct {
	profile        preflight.InstallProfile
	postgresqlPlan preflight.PostgreSQLPlan
	sample         bool
}

type preflightCollector func(context.Context, preflight.CollectorOptions) (preflight.SystemFacts, error)

func preflightCommand(args []string) error {
	return runPreflight(args, os.Stdout, preflight.NewCollector().Collect)
}

func runPreflight(args []string, stdout io.Writer, collect preflightCollector) error {
	options, err := parsePreflightOptions(args)
	if err != nil {
		return err
	}

	var facts preflight.SystemFacts
	if options.sample {
		facts = samplePreflightFacts(options.profile, options.postgresqlPlan)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		facts, err = collect(ctx, preflight.CollectorOptions{
			Profile:        options.profile,
			PostgreSQLPlan: options.postgresqlPlan,
		})
		if err != nil {
			return err
		}
	}

	report := preflight.Run(facts)
	writePreflightReport(stdout, report)
	if !report.Ready() {
		return fmt.Errorf("preflight has blocking failures")
	}
	return nil
}

func parsePreflightOptions(args []string) (preflightOptions, error) {
	if len(args) > 0 && args[0] == "sample" {
		args = append([]string{"--sample"}, args[1:]...)
	}

	flags := flag.NewFlagSet("preflight", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	var profileValue string
	var postgresqlValue string
	var options preflightOptions
	flags.StringVar(&profileValue, "profile", string(preflight.ProfileSharedHosting), "install profile: shared-hosting or single-user")
	flags.StringVar(&postgresqlValue, "postgresql", string(preflight.PostgreSQLPlanInstall), "PostgreSQL plan: install or external")
	flags.BoolVar(&options.sample, "sample", false, "use built-in sample facts")

	if err := flags.Parse(args); err != nil {
		return preflightOptions{}, err
	}
	if flags.NArg() != 0 {
		return preflightOptions{}, fmt.Errorf("unexpected preflight arguments")
	}

	profile, err := preflight.ParseInstallProfile(profileValue)
	if err != nil {
		return preflightOptions{}, err
	}
	postgresqlPlan, err := preflight.ParsePostgreSQLPlan(postgresqlValue)
	if err != nil {
		return preflightOptions{}, err
	}
	options.profile = profile
	options.postgresqlPlan = postgresqlPlan
	return options, nil
}

func samplePreflightFacts(profile preflight.InstallProfile, postgresqlPlan preflight.PostgreSQLPlan) preflight.SystemFacts {
	ramMB := preflight.MinimumSharedHostingRAMMB
	diskGB := preflight.MinimumSharedHostingDiskGB
	swapMB := preflight.MinimumSharedHostingSwapMB
	if profile == preflight.ProfileSingleUser {
		ramMB = preflight.MinimumSingleUserRAMMB
		diskGB = preflight.MinimumSingleUserDiskGB
		swapMB = preflight.MinimumSingleUserSwapMB
	}

	return preflight.SystemFacts{
		OS:             osdetect.OSRelease{ID: "ubuntu", VersionID: "24.04", Name: "Ubuntu 24.04 LTS"},
		Profile:        profile,
		CPUCores:       preflight.MinimumCPUCores,
		RAMMB:          ramMB,
		DiskGB:         diskGB,
		SwapMB:         swapMB,
		IsRoot:         true,
		HasSystemd:     true,
		PortsAvailable: map[int]bool{80: true, 443: true},
		PostgreSQLPlan: postgresqlPlan,
	}
}

func writePreflightReport(stdout io.Writer, report preflight.Report) {
	for _, check := range report.Checks {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", check.Status, check.Name, check.Message)
	}
}
