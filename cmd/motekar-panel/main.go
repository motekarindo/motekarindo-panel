package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/motekar/motekar-panel/internal/agent"
	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/buildinfo"
	"github.com/motekar/motekar-panel/internal/config"
	"github.com/motekar/motekar-panel/internal/database"
	"github.com/motekar/motekar-panel/internal/logging"
	"github.com/motekar/motekar-panel/internal/rbac"
	"github.com/motekar/motekar-panel/internal/server"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "motekar-panel: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "bootstrap":
		return bootstrap(args[1:])
	case "serve":
		return serve()
	case "migrate":
		return migrate(args[1:])
	case "version":
		fmt.Println(buildinfo.String())
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func migrate(args []string) error {
	subcommand := "up"
	if len(args) > 0 {
		subcommand = args[0]
	}
	if subcommand != "up" {
		return fmt.Errorf("unknown migrate command %q", subcommand)
	}

	cfg, err := config.LoadPanel()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	migrations, err := database.LoadMigrations(os.DirFS(filepath.Clean(cfg.MigrationsDir)))
	if err != nil {
		return err
	}

	ran, err := database.NewRunner(database.NewSQLStore(db)).Up(ctx, migrations)
	if err != nil {
		return err
	}
	for _, migration := range ran {
		fmt.Printf("applied %06d_%s\n", migration.Version, migration.Name)
	}
	if len(ran) == 0 {
		fmt.Println("no migrations to apply")
	}
	return nil
}

func serve() error {
	cfg, err := config.LoadPanel()
	if err != nil {
		return err
	}

	log := logging.New(os.Stdout, cfg.LogLevel)
	connectContext, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := database.OpenPostgres(connectContext, cfg.DatabaseURL)
	cancelConnect()
	if err != nil {
		return err
	}
	defer db.Close()

	agentClient := agent.NewUnixClient(cfg.AgentSocketPath, 2*time.Second)
	sessions := auth.NewSessionService(auth.NewSQLSessionStore(db))
	authorization := rbac.NewAuthorizer(rbac.NewSQLPermissionChecker(db))
	auditStore := audit.NewSQLStore(db)
	app := server.New(server.Config{
		Version:       buildinfo.Info(),
		Sessions:      sessions,
		Authorization: authorization,
		AuditEvents:   auditStore,
		AuditRecorder: audit.NewWriter(auditStore),
		AuditError: func(err error) {
			log.Error("audit event write failed", "error", err.Error())
		},
		SecureCookies: cfg.Environment == "production",
		Ready: func(ctx context.Context) error {
			if err := db.PingContext(ctx); err != nil {
				return fmt.Errorf("database unavailable: %w", err)
			}
			return agentClient.Health(ctx)
		},
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("panel server starting", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errs:
		return err
	case <-stop:
		log.Info("panel server stopping")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(ctx)
	}
}
