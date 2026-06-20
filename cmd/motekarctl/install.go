package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/motekar/motekar-panel/internal/installer"
	"github.com/motekar/motekar-panel/internal/preflight"
)

type installPlanOptions struct {
	profile        preflight.InstallProfile
	webServer      string
	postgresqlPlan preflight.PostgreSQLPlan
	sample         bool
}

func installCommand(args []string) error {
	if len(args) == 0 || args[0] != "plan" {
		return fmt.Errorf("usage: motekarctl install plan --profile <profile> --web-server <nginx|apache> --postgresql <install|external>")
	}
	return runInstallPlan(args[1:], os.Stdout, preflight.NewCollector().Collect)
}

func runInstallPlan(args []string, stdout io.Writer, collect preflightCollector) error {
	options, err := parseInstallPlanOptions(args)
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
	plan, err := installer.BuildPlan(installer.PlanInput{
		Profile:        options.profile,
		WebServer:      options.webServer,
		PostgreSQLPlan: options.postgresqlPlan,
		Preflight:      report,
	})
	if err != nil {
		return err
	}

	writeInstallPlan(stdout, plan)
	if !plan.Ready() {
		return fmt.Errorf("install plan has blocking preflight failures")
	}
	return nil
}

func parseInstallPlanOptions(args []string) (installPlanOptions, error) {
	flags := flag.NewFlagSet("install plan", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	var profileValue string
	var postgresqlValue string
	var options installPlanOptions
	flags.StringVar(&profileValue, "profile", string(preflight.ProfileSharedHosting), "install profile: shared-hosting or single-user")
	flags.StringVar(&options.webServer, "web-server", "", "web server: nginx or apache")
	flags.StringVar(&postgresqlValue, "postgresql", string(preflight.PostgreSQLPlanInstall), "PostgreSQL plan: install or external")
	flags.BoolVar(&options.sample, "sample", false, "use built-in sample facts")

	if err := flags.Parse(args); err != nil {
		return installPlanOptions{}, err
	}
	if flags.NArg() != 0 {
		return installPlanOptions{}, fmt.Errorf("unexpected install plan arguments")
	}

	profile, err := preflight.ParseInstallProfile(profileValue)
	if err != nil {
		return installPlanOptions{}, err
	}
	postgresqlPlan, err := preflight.ParsePostgreSQLPlan(postgresqlValue)
	if err != nil {
		return installPlanOptions{}, err
	}

	options.profile = profile
	options.postgresqlPlan = postgresqlPlan
	return options, nil
}

func writeInstallPlan(stdout io.Writer, plan installer.Plan) {
	fmt.Fprintln(stdout, "Motekar Panel install plan")
	fmt.Fprintln(stdout, "mode: dry-run")
	fmt.Fprintf(stdout, "profile: %s\n", plan.Profile)
	fmt.Fprintf(stdout, "web_server: %s\n", plan.WebServer)
	fmt.Fprintf(stdout, "postgresql: %s\n", plan.PostgreSQLPlan)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "preflight:")
	writePreflightReport(stdout, plan.Preflight)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "planned_actions:")
	for _, action := range plan.Actions {
		prefix := "READ"
		if action.ChangesHost {
			prefix = "WOULD_CHANGE"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", prefix, action.ID, action.Description)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "No changes were made.")
}
