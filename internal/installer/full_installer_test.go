package installer

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/preflight"
	"github.com/motekar/motekar-panel/internal/settings"
)

type recordingRunner struct {
	calls [][]string
	err   error
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if r.err != nil {
		return nil, r.err
	}
	return nil, nil
}

func TestFullInstallerSupportsEveryPlannedAction(t *testing.T) {
	plan, err := BuildPlan(PlanInput{
		Profile:        preflight.ProfileSingleUser,
		WebServer:      "nginx",
		PostgreSQLPlan: preflight.PostgreSQLPlanInstall,
		Preflight:      readyReport(),
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	runner := &recordingRunner{}
	installer := NewFullInstaller(runner, nil, nil)
	installer.WebServer = "nginx"
	installer.EtcDir = t.TempDir()
	installer.SystemdDir = t.TempDir()
	installer.SelectWebServer = func(_ context.Context, _ *sql.DB, _ string) error { return nil }
	installer.CreateFirstAdmin = func(_ context.Context, _ *sql.DB, _ auth.BootstrapInput) error { return nil }
	installer.RunMigrations = func(_ context.Context, _ *sql.DB, _ fs.FS) error { return nil }

	for _, action := range plan.Actions {
		if err := installer.Execute(context.Background(), action); err != nil {
			t.Fatalf("Execute(%s): %v", action.ID, err)
		}
	}
}

func TestFullInstallerRejectsUnsupportedAction(t *testing.T) {
	installer := NewFullInstaller(&recordingRunner{}, nil, nil)
	err := installer.Execute(context.Background(), Action{ID: "bogus.action"})
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFullInstallerInstallPostgresBuildsDatabaseURL(t *testing.T) {
	runner := &recordingRunner{}
	installer := NewFullInstaller(runner, nil, nil)
	installer.WebServer = "nginx"

	if err := installer.Execute(context.Background(), Action{ID: "postgresql.install"}); err != nil {
		t.Fatalf("postgresql.install: %v", err)
	}

	url := installer.ResolvedDatabaseURL()
	if !strings.HasPrefix(url, "postgres://motekar:") {
		t.Fatalf("database URL = %q", url)
	}
	if !strings.HasSuffix(url, "@127.0.0.1:5432/motekar_panel?sslmode=disable") {
		t.Fatalf("database URL = %q", url)
	}

	foundApt := false
	for _, call := range runner.calls {
		if call[0] == "apt-get" && call[1] == "install" {
			foundApt = true
		}
	}
	if !foundApt {
		t.Fatalf("expected apt-get install call, got %v", runner.calls)
	}
}

func TestFullInstallerInstallPostgresRunsAsPostgresUser(t *testing.T) {
	runner := &recordingRunner{}
	installer := NewFullInstaller(runner, nil, nil)
	installer.WebServer = "nginx"

	if err := installer.Execute(context.Background(), Action{ID: "postgresql.install"}); err != nil {
		t.Fatalf("postgresql.install: %v", err)
	}

	ranAsPostgres := false
	for _, call := range runner.calls {
		if call[0] == "runuser" && call[1] == "-u" && call[2] == "postgres" {
			ranAsPostgres = true
		}
	}
	if !ranAsPostgres {
		t.Fatalf("expected runuser calls, got %v", runner.calls)
	}
}

func TestFullInstallerExternalDatabaseRequiresURL(t *testing.T) {
	installer := NewFullInstaller(&recordingRunner{}, nil, nil)
	err := installer.Execute(context.Background(), Action{ID: "postgresql.external"})
	if err == nil {
		t.Fatal("expected database URL error")
	}
}

func TestFullInstallerExternalDatabaseUsesURL(t *testing.T) {
	installer := NewFullInstaller(&recordingRunner{}, nil, nil)
	installer.DatabaseURL = "postgres://ext:secret@host:5432/db?sslmode=disable"
	if err := installer.Execute(context.Background(), Action{ID: "postgresql.external"}); err != nil {
		t.Fatalf("postgresql.external: %v", err)
	}
	if installer.ResolvedDatabaseURL() != "postgres://ext:secret@host:5432/db?sslmode=disable" {
		t.Fatalf("database URL = %q", installer.ResolvedDatabaseURL())
	}
}

func TestFullInstallerInstallsSelectedWebServer(t *testing.T) {
	runner := &recordingRunner{}
	installer := NewFullInstaller(runner, nil, nil)
	installer.WebServer = "apache"
	if err := installer.Execute(context.Background(), Action{ID: "webserver.install"}); err != nil {
		t.Fatalf("webserver.install: %v", err)
	}

	installedApache := false
	for _, call := range runner.calls {
		if call[0] == "apt-get" && call[1] == "install" && contains(call, "apache2") {
			installedApache = true
		}
	}
	if !installedApache {
		t.Fatalf("expected apache2 install, got %v", runner.calls)
	}
}

func TestFullInstallerWritesEnvAndSystemdUnits(t *testing.T) {
	etcDir := t.TempDir()
	systemdDir := t.TempDir()

	installer := NewFullInstaller(&recordingRunner{}, nil, nil)
	installer.WebServer = "nginx"
	installer.EtcDir = etcDir
	installer.SystemdDir = systemdDir
	installer.BinDir = "/opt/motekar/bin"
	installer.AgentSocketPath = "/run/motekar/agent.sock"
	installer.PanelAddr = "127.0.0.1:8080"
	installer.Environment = "production"
	installer.dbURL = "postgres://motekar:secret@127.0.0.1:5432/motekar_panel?sslmode=disable"

	if err := installer.Execute(context.Background(), Action{ID: "systemd.services"}); err != nil {
		t.Fatalf("systemd.services: %v", err)
	}

	for _, name := range []string{"panel.env", "agent.env"} {
		path := filepath.Join(etcDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if len(content) == 0 {
			t.Fatalf("%s is empty", name)
		}
	}

	panelEnv := readFile(t, filepath.Join(etcDir, "panel.env"))
	if !strings.Contains(panelEnv, "MOTEKAR_DATABASE_URL=postgres://motekar:secret@127.0.0.1:5432/motekar_panel?sslmode=disable") {
		t.Fatalf("panel.env missing database URL:\n%s", panelEnv)
	}
	if !strings.Contains(panelEnv, "MOTEKAR_AGENT_SOCKET=/run/motekar/agent.sock") {
		t.Fatalf("panel.env missing agent socket:\n%s", panelEnv)
	}

	agentEnv := readFile(t, filepath.Join(etcDir, "agent.env"))
	if !strings.Contains(agentEnv, "MOTEKAR_AGENT_SOCKET=/run/motekar/agent.sock") {
		t.Fatalf("agent.env missing agent socket:\n%s", agentEnv)
	}

	panelUnit := readFile(t, filepath.Join(systemdDir, "motekar-panel.service"))
	if !strings.Contains(panelUnit, "/opt/motekar/bin/motekar-panel serve") {
		t.Fatalf("panel unit missing ExecStart:\n%s", panelUnit)
	}
	if !strings.Contains(panelUnit, filepath.Join(etcDir, "panel.env")) {
		t.Fatalf("panel unit missing EnvironmentFile:\n%s", panelUnit)
	}

	agentUnit := readFile(t, filepath.Join(systemdDir, "motekar-agent.service"))
	if !strings.Contains(agentUnit, "/opt/motekar/bin/motekar-agent serve") {
		t.Fatalf("agent unit missing ExecStart:\n%s", agentUnit)
	}
	if !strings.Contains(agentUnit, "RuntimeDirectory=motekar-panel") {
		t.Fatalf("agent unit missing RuntimeDirectory:\n%s", agentUnit)
	}
}

func TestFullInstallerSelectsImmutableWebServer(t *testing.T) {
	var selected []string
	installer := NewFullInstaller(&recordingRunner{}, nil, nil)
	installer.WebServer = "nginx"
	installer.dbURL = "postgres://motekar:secret@127.0.0.1:5432/motekar_panel?sslmode=disable"
	installer.SelectWebServer = func(_ context.Context, _ *sql.DB, value string) error {
		selected = append(selected, value)
		return nil
	}

	if err := installer.Execute(context.Background(), Action{ID: "settings.webserver"}); err != nil {
		t.Fatalf("settings.webserver: %v", err)
	}
	if len(selected) != 1 || selected[0] != "nginx" {
		t.Fatalf("selected = %v", selected)
	}
}

func TestFullInstallerBootstrapAdmin(t *testing.T) {
	var input auth.BootstrapInput
	installer := NewFullInstaller(&recordingRunner{}, nil, nil)
	installer.WebServer = "nginx"
	installer.dbURL = "postgres://motekar:secret@127.0.0.1:5432/motekar_panel?sslmode=disable"
	installer.AdminEmail = "owner@example.com"
	installer.AdminDisplayName = "Owner"
	installer.AdminPassword = "correct-horse-battery"
	installer.EtcDir = t.TempDir()
	installer.SystemdDir = t.TempDir()
	installer.CreateFirstAdmin = func(_ context.Context, _ *sql.DB, adminInput auth.BootstrapInput) error {
		input = adminInput
		return nil
	}

	if err := installer.Execute(context.Background(), Action{ID: "systemd.services"}); err != nil {
		t.Fatalf("systemd.services with admin: %v", err)
	}
	if input.Email != "owner@example.com" || input.Password != "correct-horse-battery" {
		t.Fatalf("admin input = %#v", input)
	}
}

func TestFullInstallerDatabaseRequiredBeforeMigrate(t *testing.T) {
	installer := NewFullInstaller(&recordingRunner{}, nil, nil)
	installer.Migrations = fstest.MapFS{}
	err := installer.Execute(context.Background(), Action{ID: "database.migrate"})
	if err == nil {
		t.Fatal("expected database URL error")
	}
}

func TestFullInstallerRunnerErrorStopsInstall(t *testing.T) {
	runner := &recordingRunner{err: errors.New("apt failed")}
	installer := NewFullInstaller(runner, nil, nil)
	installer.WebServer = "nginx"
	if err := installer.Execute(context.Background(), Action{ID: "postgresql.install"}); err == nil {
		t.Fatal("expected apt failure")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

var _ settings.WebServer
