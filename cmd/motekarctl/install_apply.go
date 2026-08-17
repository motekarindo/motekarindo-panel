package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/config"
	"github.com/motekar/motekar-panel/internal/database"
	"github.com/motekar/motekar-panel/internal/installer"
	"github.com/motekar/motekar-panel/internal/preflight"
	"github.com/motekar/motekar-panel/internal/settings"
)

type installApplyOptions struct {
	installPlanOptions
	databaseURL string
}

func runInstallApply(args []string, stdout io.Writer, collect preflightCollector, openExecutor func(context.Context, string, string) (installer.ActionExecutor, io.Closer, error)) error {
	options, err := parseInstallApplyOptions(args)
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
	if !plan.Ready() {
		return fmt.Errorf("install plan has blocking preflight failures")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	executor, closer, err := openExecutor(ctx, options.databaseURL, string(plan.WebServer))
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}

	result, err := installer.Apply(ctx, plan, executor)
	if err != nil {
		return err
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(stdout, "skipped %d action(s) not yet supported:\n", len(result.Skipped))
		for _, action := range result.Skipped {
			fmt.Fprintf(stdout, "  - %s\n", action.ID)
		}
	}

	fmt.Fprintf(stdout, "web_server: %s\n", plan.WebServer)
	fmt.Fprintln(stdout, "Installer persisted the selected web server as an immutable server setting.")
	return nil
}

func parseInstallApplyOptions(args []string) (installApplyOptions, error) {
	flags := flag.NewFlagSet("install apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	var options installApplyOptions
	var profileValue string
	var postgresqlValue string
	flags.StringVar(&profileValue, "profile", string(preflight.ProfileSharedHosting), "install profile: shared-hosting or single-user")
	flags.StringVar(&options.webServer, "web-server", "", "web server: nginx or apache")
	flags.StringVar(&postgresqlValue, "postgresql", string(preflight.PostgreSQLPlanInstall), "PostgreSQL plan: install or external")
	flags.BoolVar(&options.sample, "sample", false, "use built-in sample facts")
	flags.StringVar(&options.databaseURL, "database-url", "", "PostgreSQL connection URL (defaults to MOTEKAR_DATABASE_URL)")

	if err := flags.Parse(args); err != nil {
		return installApplyOptions{}, err
	}
	if flags.NArg() != 0 {
		return installApplyOptions{}, fmt.Errorf("unexpected install apply arguments")
	}

	profile, err := preflight.ParseInstallProfile(profileValue)
	if err != nil {
		return installApplyOptions{}, err
	}
	postgresqlPlan, err := preflight.ParsePostgreSQLPlan(postgresqlValue)
	if err != nil {
		return installApplyOptions{}, err
	}
	options.profile = profile
	options.postgresqlPlan = postgresqlPlan
	return options, nil
}

func openInstallExecutor(ctx context.Context, databaseURL, webServer string) (installer.ActionExecutor, io.Closer, error) {
	cfg, err := config.LoadPanel()
	if err != nil {
		return nil, nil, err
	}
	url := databaseURL
	if url == "" {
		url = cfg.DatabaseURL
	}
	if url == "" {
		return nil, nil, fmt.Errorf("database URL is required; pass --database-url or set MOTEKAR_DATABASE_URL")
	}
	db, err := database.OpenPostgres(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	webServerService := settings.NewWebServerService(settings.NewSQLStore(db)).
		WithAudit(audit.NewWriter(audit.NewSQLStore(db)))
	return installer.WebServerExecutor{Service: webServerService, Value: webServer}, db, nil
}

func installApplyCommand(args []string) error {
	return runInstallApply(args, os.Stdout, preflight.NewCollector().Collect, openInstallExecutor)
}
