package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/config"
	"github.com/motekar/motekar-panel/internal/database"
	"github.com/motekar/motekar-panel/internal/installer"
	"github.com/motekar/motekar-panel/internal/preflight"
	"github.com/motekar/motekar-panel/internal/settings"
	"github.com/motekar/motekar-panel/services/migrations"
)

type installApplyOptions struct {
	installPlanOptions
	databaseURL        string
	binDir             string
	etcDir             string
	systemdDir         string
	agentSocket        string
	panelAddr          string
	environment        string
	adminEmail         string
	adminDisplayName   string
	adminPasswordStdin bool
	adminPassword      string
	noSystemd          bool
}

func runInstallApply(args []string, stdout io.Writer, collect preflightCollector, openExecutor func(context.Context, installApplyOptions) (installer.ActionExecutor, io.Closer, error)) error {
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	executor, closer, err := openExecutor(ctx, options)
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
	if options.adminEmail != "" {
		fmt.Fprintf(stdout, "admin: %s\n", options.adminEmail)
	}
	fmt.Fprintln(stdout, "Motekar Panel installed.")
	return nil
}

func parseInstallApplyOptions(args []string) (installApplyOptions, error) {
	return parseInstallApplyOptionsFromReader(args, os.Stdin)
}

func parseInstallApplyOptionsFromReader(args []string, stdin io.Reader) (installApplyOptions, error) {
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
	flags.StringVar(&options.binDir, "bin-dir", "/usr/local/bin", "directory where panel and agent binaries live")
	flags.StringVar(&options.etcDir, "etc-dir", "/etc/motekar-panel", "directory for panel and agent environment files")
	flags.StringVar(&options.systemdDir, "systemd-dir", "/etc/systemd/system", "directory for systemd unit files")
	flags.StringVar(&options.agentSocket, "agent-socket", "/run/motekar-panel/agent.sock", "agent Unix socket path")
	flags.StringVar(&options.panelAddr, "panel-addr", ":8080", "panel HTTP listen address")
	flags.StringVar(&options.environment, "environment", "production", "runtime environment label")
	flags.StringVar(&options.adminEmail, "admin-email", "", "first admin email")
	flags.StringVar(&options.adminDisplayName, "admin-display-name", "", "first admin display name")
	flags.BoolVar(&options.adminPasswordStdin, "admin-password-stdin", false, "read the first admin password from stdin")
	flags.BoolVar(&options.noSystemd, "no-systemd", false, "do not install systemd services (development only)")

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

	if options.adminEmail != "" && options.adminPasswordStdin {
		password, err := readInstallAdminPassword(stdin)
		if err != nil {
			return installApplyOptions{}, err
		}
		options.adminPassword = password
	}
	return options, nil
}

func readInstallAdminPassword(stdin io.Reader) (string, error) {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if password == "" {
		return "", fmt.Errorf("admin password cannot be empty")
	}
	return password, nil
}

func openInstallExecutor(ctx context.Context, options installApplyOptions) (installer.ActionExecutor, io.Closer, error) {
	cfg, err := config.LoadPanel()
	if err != nil {
		return nil, nil, err
	}
	url := options.databaseURL
	if url == "" {
		url = cfg.DatabaseURL
	}

	openDB := database.OpenPostgres
	if options.noSystemd {
		if url == "" {
			return nil, nil, fmt.Errorf("database URL is required; pass --database-url or set MOTEKAR_DATABASE_URL")
		}
		// Keep the previous single-action behavior for development tests.
		db, err := database.OpenPostgres(ctx, url)
		if err != nil {
			return nil, nil, err
		}
		webServerService := settings.NewWebServerService(settings.NewSQLStore(db)).
			WithAudit(audit.NewWriter(audit.NewSQLStore(db)))
		return installer.WebServerExecutor{Service: webServerService, Value: options.webServer}, db, nil
	}

	fullInstaller := installer.NewFullInstaller(nil, openDB, migrations.FS)
	fullInstaller.Profile = options.profile
	fullInstaller.WebServer = options.webServer
	fullInstaller.PostgreSQLPlan = options.postgresqlPlan
	fullInstaller.DatabaseURL = url
	fullInstaller.BinDir = options.binDir
	fullInstaller.EtcDir = options.etcDir
	fullInstaller.SystemdDir = options.systemdDir
	fullInstaller.AgentSocketPath = options.agentSocket
	fullInstaller.PanelAddr = options.panelAddr
	fullInstaller.Environment = options.environment
	fullInstaller.AdminEmail = options.adminEmail
	fullInstaller.AdminDisplayName = options.adminDisplayName
	fullInstaller.AdminPassword = options.adminPassword
	return fullInstaller, fullInstaller, nil
}

func installApplyCommand(args []string) error {
	return runInstallApply(args, os.Stdout, preflight.NewCollector().Collect, openInstallExecutor)
}
