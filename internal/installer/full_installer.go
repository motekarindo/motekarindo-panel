package installer

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/database"
	"github.com/motekar/motekar-panel/internal/preflight"
	"github.com/motekar/motekar-panel/internal/settings"
)

// FullInstaller executes every install plan action against the host. The
// database connection is opened lazily after the PostgreSQL action so the
// database URL is available before the settings and migration actions run.
type FullInstaller struct {
	Profile        preflight.InstallProfile
	WebServer      string
	PostgreSQLPlan preflight.PostgreSQLPlan
	DatabaseURL    string

	Runner     CommandRunner
	OpenDB     func(ctx context.Context, databaseURL string) (*sql.DB, error)
	Migrations fs.FS

	SelectWebServer  func(ctx context.Context, db *sql.DB, value string) error
	CreateFirstAdmin func(ctx context.Context, db *sql.DB, input auth.BootstrapInput) error
	RunMigrations    func(ctx context.Context, db *sql.DB, migrations fs.FS) error

	BinDir          string
	EtcDir          string
	SystemdDir      string
	AgentSocketPath string
	PanelAddr       string
	Environment     string

	AdminEmail       string
	AdminDisplayName string
	AdminPassword    string

	db        *sql.DB
	dbURL     string
	generated string
}

const (
	defaultBinDir          = "/usr/local/bin"
	defaultEtcDir          = "/etc/motekar-panel"
	defaultSystemdDir      = "/etc/systemd/system"
	defaultAgentSocketPath = "/run/motekar-panel/agent.sock"
	defaultPanelAddr       = ":8080"
	defaultEnvironment     = "production"

	panelServiceName = "motekar-panel.service"
	agentServiceName = "motekar-agent.service"
)

func NewFullInstaller(runner CommandRunner, openDB func(context.Context, string) (*sql.DB, error), migrations fs.FS) *FullInstaller {
	if runner == nil {
		runner = newExecCommandRunner()
	}
	if migrations == nil {
		migrations = os.DirFS(filepath.Clean("services/migrations"))
	}
	return &FullInstaller{
		Runner:          runner,
		OpenDB:          openDB,
		Migrations:      migrations,
		BinDir:          defaultBinDir,
		EtcDir:          defaultEtcDir,
		SystemdDir:      defaultSystemdDir,
		AgentSocketPath: defaultAgentSocketPath,
		PanelAddr:       defaultPanelAddr,
		Environment:     defaultEnvironment,
	}
}

func (i *FullInstaller) Close() error {
	if i.db != nil {
		return i.db.Close()
	}
	return nil
}

func (i *FullInstaller) ResolvedDatabaseURL() string {
	return i.dbURL
}

func (i *FullInstaller) Execute(ctx context.Context, action Action) error {
	switch action.ID {
	case "preflight.verify":
		return i.verifyPreflight(ctx)
	case "postgresql.external":
		return i.setExternalDatabase(ctx)
	case "postgresql.install":
		return i.installPostgres(ctx)
	case "webserver.install":
		return i.installWebServer(ctx)
	case "settings.webserver":
		return i.selectWebServer(ctx)
	case "database.migrate":
		return i.runMigrations(ctx)
	case "systemd.services":
		return i.installSystemdServices(ctx)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedAction, action.ID)
	}
}

func (i *FullInstaller) verifyPreflight(ctx context.Context) error {
	return nil
}

func (i *FullInstaller) setExternalDatabase(ctx context.Context) error {
	url := strings.TrimSpace(i.DatabaseURL)
	if url == "" {
		return errors.New("external PostgreSQL plan requires a database URL")
	}
	i.dbURL = url
	return nil
}

func (i *FullInstaller) installPostgres(ctx context.Context) error {
	commands := [][]string{
		{"apt-get", "update"},
		{"apt-get", "install", "-y", "-o", "Dpkg::Progress-Fancy=0", "postgresql"},
		{"systemctl", "enable", "--now", "postgresql"},
	}
	for _, command := range commands {
		if output, err := i.Runner.Run(ctx, command[0], command[1:]...); err != nil {
			return fmt.Errorf("%s failed: %w\n%s", strings.Join(command, " "), err, output)
		}
	}

	password, err := generateDatabasePassword()
	if err != nil {
		return err
	}
	i.generated = password

	roleSQL := fmt.Sprintf(
		`CREATE ROLE %s LOGIN PASSWORD '%s';`,
		postgresRole,
		strings.ReplaceAll(password, "'", "''"),
	)
	if output, err := i.Runner.Run(ctx, "runuser", "-u", "postgres", "--", "psql", "-v", "ON_ERROR_STOP=1", "-c", roleSQL); err != nil {
		return fmt.Errorf("create PostgreSQL role failed: %w\n%s", err, output)
	}
	if output, err := i.Runner.Run(ctx, "runuser", "-u", "postgres", "--", "psql", "-v", "ON_ERROR_STOP=1", "-c", fmt.Sprintf(`CREATE DATABASE %s OWNER %s;`, postgresDatabase, postgresRole)); err != nil {
		return fmt.Errorf("create PostgreSQL database failed: %w\n%s", err, output)
	}

	i.dbURL = buildDatabaseURL(postgresRole, password, postgresDatabase)
	return nil
}

const (
	postgresRole     = "motekar"
	postgresDatabase = "motekar_panel"
)

func buildDatabaseURL(role, password, database string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@127.0.0.1:5432/%s?sslmode=disable",
		url.PathEscape(role),
		url.QueryEscape(password),
		database,
	)
}

func generateDatabasePassword() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate database password: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func (i *FullInstaller) installWebServer(ctx context.Context) error {
	packageName := "nginx"
	if i.WebServer == "apache" {
		packageName = "apache2"
	}
	commands := [][]string{
		{"apt-get", "install", "-y", "-o", "Dpkg::Progress-Fancy=0", packageName},
		{"systemctl", "enable", "--now", packageName},
	}
	for _, command := range commands {
		if output, err := i.Runner.Run(ctx, command[0], command[1:]...); err != nil {
			return fmt.Errorf("%s failed: %w\n%s", strings.Join(command, " "), err, output)
		}
	}
	return nil
}

func (i *FullInstaller) database(ctx context.Context) (*sql.DB, error) {
	if i.db != nil {
		return i.db, nil
	}
	if i.dbURL == "" {
		return nil, errors.New("database URL is not available yet")
	}
	db, err := i.OpenDB(ctx, i.dbURL)
	if err != nil {
		return nil, err
	}
	i.db = db
	return db, nil
}

func (i *FullInstaller) selectWebServer(ctx context.Context) error {
	if i.SelectWebServer != nil {
		return i.SelectWebServer(ctx, nil, i.WebServer)
	}
	db, err := i.database(ctx)
	if err != nil {
		return err
	}
	service := settings.NewWebServerService(settings.NewSQLStore(db)).
		WithAudit(audit.NewWriter(audit.NewSQLStore(db)))
	return (WebServerExecutor{Service: service, Value: i.WebServer}).Execute(ctx, Action{ID: "settings.webserver"})
}

func (i *FullInstaller) runMigrations(ctx context.Context) error {
	if i.RunMigrations != nil {
		return i.RunMigrations(ctx, nil, i.Migrations)
	}
	db, err := i.database(ctx)
	if err != nil {
		return err
	}
	migrations, err := database.LoadMigrations(i.Migrations)
	if err != nil {
		return err
	}
	_, err = database.NewRunner(database.NewSQLStore(db)).Up(ctx, migrations)
	return err
}

func (i *FullInstaller) installSystemdServices(ctx context.Context) error {
	panelEnv := i.panelEnv()
	agentEnv := i.agentEnv()
	panelUnit := i.panelUnit()
	agentUnit := i.agentUnit()

	if err := os.MkdirAll(i.EtcDir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", i.EtcDir, err)
	}
	envFiles := map[string]string{
		"panel.env": panelEnv,
		"agent.env": agentEnv,
	}
	for name, content := range envFiles {
		path := filepath.Join(i.EtcDir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	unitFiles := map[string]string{
		panelServiceName: panelUnit,
		agentServiceName: agentUnit,
	}
	for name, content := range unitFiles {
		path := filepath.Join(i.SystemdDir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	for _, command := range [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", "motekar-panel.service", "motekar-agent.service"},
	} {
		if output, err := i.Runner.Run(ctx, command[0], command[1:]...); err != nil {
			return fmt.Errorf("%s failed: %w\n%s", strings.Join(command, " "), err, output)
		}
	}

	if i.AdminEmail != "" {
		return i.bootstrapAdmin(ctx)
	}
	return nil
}

func (i *FullInstaller) bootstrapAdmin(ctx context.Context) error {
	input := auth.BootstrapInput{
		Email:       i.AdminEmail,
		DisplayName: i.AdminDisplayName,
		Password:    i.AdminPassword,
	}
	if i.CreateFirstAdmin != nil {
		return i.CreateFirstAdmin(ctx, nil, input)
	}
	db, err := i.database(ctx)
	if err != nil {
		return err
	}
	_, err = auth.NewBootstrapService(auth.NewSQLBootstrapStore(db)).CreateFirstAdmin(ctx, input)
	return err
}
