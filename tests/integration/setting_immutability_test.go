package integration_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/database"
	"github.com/motekar/motekar-panel/internal/installer"
	"github.com/motekar/motekar-panel/internal/settings"
)

func TestWebServerSettingImmutableAndAudited(t *testing.T) {
	databaseURL := os.Getenv("MOTEKAR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MOTEKAR_TEST_DATABASE_URL is not set")
	}
	if os.Getenv("MOTEKAR_TEST_ALLOW_DATABASE") != "1" {
		t.Fatal("set MOTEKAR_TEST_ALLOW_DATABASE=1 to run PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	controlDB, err := database.OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = controlDB.Close() })

	var databaseName string
	if err := controlDB.QueryRowContext(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		t.Fatalf("read database name: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(databaseName), "_test") {
		t.Fatalf("refusing to use database %q: disposable database name must end with _test", databaseName)
	}

	schemaName := fmt.Sprintf("motekar_test_%d", time.Now().UnixNano())
	if _, err := controlDB.ExecContext(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatalf("create isolated test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = controlDB.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	})

	testDatabaseURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse PostgreSQL URL: %v", err)
	}
	query := testDatabaseURL.Query()
	query.Set("search_path", schemaName)
	testDatabaseURL.RawQuery = query.Encode()

	db, err := database.OpenPostgres(ctx, testDatabaseURL.String())
	if err != nil {
		t.Fatalf("open isolated PostgreSQL schema: %v", err)
	}
	defer db.Close()

	migrations, err := database.LoadMigrations(os.DirFS(filepath.Join("..", "..", "services", "migrations")))
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	runner := database.NewRunner(database.NewSQLStore(db))
	if _, err := runner.Up(ctx, migrations); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	service := settings.NewWebServerService(settings.NewSQLStore(db)).
		WithAudit(audit.NewWriter(audit.NewSQLStore(db)))

	if _, err := service.Select(ctx, "nginx"); err != nil {
		t.Fatalf("first Select: %v", err)
	}
	assertCount(t, ctx, db, `
		SELECT count(*) FROM server_settings
		WHERE key = 'web_server' AND value = 'nginx' AND is_immutable
	`, 1)
	assertCount(t, ctx, db, `
		SELECT count(*) FROM audit_events
		WHERE action = $1
		  AND target_type = 'server_setting'
		  AND target_id = 'web_server'
		  AND metadata->>'value' = 'nginx'
	`, 1, audit.ActionWebServerSelected)

	if _, err := service.Select(ctx, "apache"); !errors.Is(err, settings.ErrWebServerAlreadySelected) {
		t.Fatalf("second Select error = %v, want %v", err, settings.ErrWebServerAlreadySelected)
	}
	assertCount(t, ctx, db, `
		SELECT count(*) FROM server_settings
		WHERE key = 'web_server' AND value = 'nginx' AND is_immutable
	`, 1)
	assertCount(t, ctx, db, `
		SELECT count(*) FROM audit_events
		WHERE action = $1
		  AND target_type = 'server_setting'
		  AND target_id = 'web_server'
		  AND metadata->>'value' = 'apache'
		  AND metadata->>'current' = 'nginx'
	`, 1, audit.ActionWebServerChangeDenied)

	directStore := settings.NewSQLStore(db)
	if err := directStore.Save(ctx, settings.Setting{
		Key:         settings.WebServerSettingKey,
		Value:       "apache",
		IsImmutable: true,
	}); !errors.Is(err, settings.ErrWebServerAlreadySelected) {
		t.Fatalf("direct immutable Save error = %v, want %v", err, settings.ErrWebServerAlreadySelected)
	}
	assertCount(t, ctx, db, `
		SELECT count(*) FROM server_settings
		WHERE key = 'web_server' AND value = 'nginx' AND is_immutable
	`, 1)
}

func TestWebServerExecutorPersistsDuringApply(t *testing.T) {
	databaseURL := os.Getenv("MOTEKAR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MOTEKAR_TEST_DATABASE_URL is not set")
	}
	if os.Getenv("MOTEKAR_TEST_ALLOW_DATABASE") != "1" {
		t.Fatal("set MOTEKAR_TEST_ALLOW_DATABASE=1 to run PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	controlDB, err := database.OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = controlDB.Close() })

	var databaseName string
	if err := controlDB.QueryRowContext(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		t.Fatalf("read database name: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(databaseName), "_test") {
		t.Fatalf("refusing to use database %q: disposable database name must end with _test", databaseName)
	}

	schemaName := fmt.Sprintf("motekar_test_%d", time.Now().UnixNano())
	if _, err := controlDB.ExecContext(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatalf("create isolated test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = controlDB.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	})

	testDatabaseURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse PostgreSQL URL: %v", err)
	}
	query := testDatabaseURL.Query()
	query.Set("search_path", schemaName)
	testDatabaseURL.RawQuery = query.Encode()

	db, err := database.OpenPostgres(ctx, testDatabaseURL.String())
	if err != nil {
		t.Fatalf("open isolated PostgreSQL schema: %v", err)
	}
	defer db.Close()

	migrations, err := database.LoadMigrations(os.DirFS(filepath.Join("..", "..", "services", "migrations")))
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	runner := database.NewRunner(database.NewSQLStore(db))
	if _, err := runner.Up(ctx, migrations); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	service := settings.NewWebServerService(settings.NewSQLStore(db)).
		WithAudit(audit.NewWriter(audit.NewSQLStore(db)))
	executor := installer.WebServerExecutor{Service: service, Value: "nginx"}
	if err := executor.Execute(ctx, installer.Action{ID: "settings.webserver", ChangesHost: true}); err != nil {
		t.Fatalf("execute settings.webserver: %v", err)
	}

	var value string
	var immutable bool
	if err := db.QueryRowContext(ctx, `
		SELECT value, is_immutable
		FROM server_settings
		WHERE key = 'web_server'
	`).Scan(&value, &immutable); err != nil {
		t.Fatalf("read persisted web server: %v", err)
	}
	if value != "nginx" || !immutable {
		t.Fatalf("persisted web server = (%q, %t), want nginx and immutable", value, immutable)
	}
	assertCount(t, ctx, db, `
		SELECT count(*) FROM audit_events
		WHERE action = $1
		  AND target_type = 'server_setting'
		  AND target_id = 'web_server'
		  AND metadata->>'value' = 'nginx'
	`, 1, audit.ActionWebServerSelected)
}
