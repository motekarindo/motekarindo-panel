package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/database"
	"github.com/motekar/motekar-panel/internal/rbac"
	"github.com/motekar/motekar-panel/internal/server"
)

func TestPostgresMigrationsAndCoreSchema(t *testing.T) {
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
	if len(migrations) != 4 {
		t.Fatalf("loaded %d migrations, want 4", len(migrations))
	}

	runner := database.NewRunner(database.NewSQLStore(db))
	ran, err := runner.Up(ctx, migrations[:3])
	if err != nil {
		t.Fatalf("apply pre-audit-hardening migrations: %v", err)
	}
	if len(ran) != 3 {
		t.Fatalf("applied %d pre-audit-hardening migrations, want 3", len(ran))
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO audit_events (id, action, target_type, target_id, metadata)
		VALUES (
			'60000000-0000-4000-8000-000000000001',
			'auth.bootstrap_admin.created',
			'user',
			'legacy-admin',
			'{"email":"legacy@example.com","source":"bootstrap"}'::jsonb
		)
	`); err != nil {
		t.Fatalf("insert legacy bootstrap audit event: %v", err)
	}

	ran, err = runner.Up(ctx, migrations)
	if err != nil {
		t.Fatalf("apply audit hardening migration: %v", err)
	}
	if len(ran) != 1 {
		t.Fatalf("applied %d audit hardening migrations, want 1", len(ran))
	}
	assertCount(t, ctx, db, `
		SELECT count(*) FROM audit_events
		WHERE id = '60000000-0000-4000-8000-000000000001'
		  AND metadata = '{"source":"bootstrap"}'::jsonb
	`, 1)

	ran, err = runner.Up(ctx, migrations)
	if err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	if len(ran) != 0 {
		t.Fatalf("reapplied %d migrations, want none", len(ran))
	}

	assertCount(t, ctx, db, `SELECT count(*) FROM schema_migrations`, 4)
	assertCount(t, ctx, db, `SELECT count(*) FROM roles`, 4)
	assertCount(t, ctx, db, `SELECT count(*) FROM permissions`, 13)
	assertRolePermissions(t, ctx, db, "owner",
		"accounts:manage", "agent:execute", "audit:read", "billing:manage",
		"databases:manage", "dns:manage", "jobs:manage", "mail:manage",
		"resellers:manage", "settings:manage", "system:admin", "users:manage",
		"websites:manage",
	)
	assertRolePermissions(t, ctx, db, "admin",
		"accounts:manage", "agent:execute", "audit:read", "databases:manage",
		"dns:manage", "jobs:manage", "mail:manage", "settings:manage",
		"users:manage", "websites:manage",
	)
	assertRolePermissions(t, ctx, db, "reseller",
		"accounts:manage", "databases:manage", "dns:manage", "mail:manage",
		"websites:manage",
	)
	assertRolePermissions(t, ctx, db, "customer",
		"databases:manage", "dns:manage", "mail:manage", "websites:manage",
	)

	var webServerValue string
	var webServerImmutable bool
	if err := db.QueryRowContext(ctx, `
		SELECT value, is_immutable
		FROM server_settings
		WHERE key = 'web_server'
	`).Scan(&webServerValue, &webServerImmutable); err != nil {
		t.Fatalf("read web server setting: %v", err)
	}
	if webServerValue != "" || !webServerImmutable {
		t.Fatalf("web server setting = (%q, %t), want empty and immutable", webServerValue, webServerImmutable)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin smoke transaction: %v", err)
	}
	defer tx.Rollback()

	const userID = "30000000-0000-4000-8000-000000000001"
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name)
		VALUES ($1, 'integration@example.test', 'not-a-real-hash', 'Integration User')
	`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO server_nodes (id, name, os_family, os_version)
		VALUES ('30000000-0000-4000-8000-000000000002', 'integration-node', 'ubuntu', '24.04')
	`); err != nil {
		t.Fatalf("insert server node: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO jobs (id, type, status, payload)
		VALUES ('30000000-0000-4000-8000-000000000003', 'integration.smoke', 'queued', '{"source":"integration"}')
	`); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events (id, actor_user_id, action, target_type, target_id)
		VALUES ('30000000-0000-4000-8000-000000000004', $1, 'integration.smoke', 'job', '30000000-0000-4000-8000-000000000003')
	`, userID); err != nil {
		t.Fatalf("insert audit event: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, expires_at)
		VALUES ('30000000-0000-4000-8000-000000000005', $1, 'integration-token-hash', now() + interval '1 hour')
	`, userID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE server_settings SET value = 'nginx' WHERE key = 'web_server'`); err != nil {
		t.Fatalf("store web server setting: %v", err)
	}

	var email string
	if err := tx.QueryRowContext(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if email != "integration@example.test" {
		t.Fatalf("user email = %q, want integration@example.test", email)
	}
	assertCount(t, ctx, tx, `SELECT count(*) FROM sessions WHERE user_id = '30000000-0000-4000-8000-000000000001'`, 1)
	assertCount(t, ctx, tx, `SELECT count(*) FROM server_nodes WHERE name = 'integration-node'`, 1)
	assertCount(t, ctx, tx, `SELECT count(*) FROM jobs WHERE type = 'integration.smoke' AND status = 'queued'`, 1)
	assertCount(t, ctx, tx, `SELECT count(*) FROM audit_events WHERE action = 'integration.smoke'`, 1)
	assertCount(t, ctx, tx, `SELECT count(*) FROM server_settings WHERE key = 'web_server' AND value = 'nginx' AND is_immutable`, 1)
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback smoke transaction: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		ALTER TABLE audit_events
		ADD CONSTRAINT reject_bootstrap_audit
		CHECK (action <> 'auth.bootstrap_admin.created' OR target_id <> '30000000-0000-4000-8000-000000000009')
	`); err != nil {
		t.Fatalf("install audit failure constraint: %v", err)
	}
	failingService := newTestBootstrapService(db, "30000000-0000-4000-8000-000000000009")
	_, err = failingService.CreateFirstAdmin(ctx, auth.BootstrapInput{
		Email:       "failed@example.com",
		DisplayName: "Failed Owner",
		Password:    "correct horse battery staple",
	})
	if err == nil {
		t.Fatal("expected bootstrap to fail when audit insert is rejected")
	}
	assertCount(t, ctx, db, `SELECT count(*) FROM users`, 0)
	assertCount(t, ctx, db, `SELECT count(*) FROM user_roles`, 0)
	assertCount(t, ctx, db, `SELECT count(*) FROM audit_events`, 1)
	if _, err := db.ExecContext(ctx, `ALTER TABLE audit_events DROP CONSTRAINT reject_bootstrap_audit`); err != nil {
		t.Fatalf("remove audit failure constraint: %v", err)
	}

	type bootstrapResult struct {
		admin auth.BootstrapAdmin
		err   error
	}
	start := make(chan struct{})
	results := make(chan bootstrapResult, 2)
	requests := []struct {
		id    string
		input auth.BootstrapInput
	}{
		{
			id: "30000000-0000-4000-8000-000000000010",
			input: auth.BootstrapInput{
				Email:       " OWNER@Example.COM ",
				DisplayName: " Owner ",
				Password:    "correct horse battery staple",
			},
		},
		{
			id: "30000000-0000-4000-8000-000000000011",
			input: auth.BootstrapInput{
				Email:       " SECOND@Example.COM ",
				DisplayName: " Second Owner ",
				Password:    "correct horse battery staple",
			},
		},
	}
	for _, request := range requests {
		go func() {
			<-start
			admin, err := newTestBootstrapService(db, request.id).CreateFirstAdmin(ctx, request.input)
			results <- bootstrapResult{admin: admin, err: err}
		}()
	}
	close(start)

	var admin auth.BootstrapAdmin
	duplicateErrors := 0
	for range requests {
		result := <-results
		switch {
		case result.err == nil:
			admin = result.admin
		case errors.Is(result.err, auth.ErrAdminAlreadyExists):
			duplicateErrors++
		default:
			t.Fatalf("concurrent bootstrap returned unexpected error: %v", result.err)
		}
	}
	if admin.ID == "" || duplicateErrors != 1 {
		t.Fatalf("concurrent bootstrap created admin %#v with %d duplicate errors", admin, duplicateErrors)
	}
	if ok, err := auth.VerifyPassword("correct horse battery staple", admin.PasswordHash); err != nil || !ok {
		t.Fatalf("verify first admin password: ok=%t err=%v", ok, err)
	}

	_, err = newTestBootstrapService(db, "30000000-0000-4000-8000-000000000012").CreateFirstAdmin(ctx, auth.BootstrapInput{
		Email:       "third@example.com",
		DisplayName: "Third Owner",
		Password:    "correct horse battery staple",
	})
	if !errors.Is(err, auth.ErrAdminAlreadyExists) {
		t.Fatalf("subsequent bootstrap error = %v, want %v", err, auth.ErrAdminAlreadyExists)
	}

	assertCount(t, ctx, db, `SELECT count(*) FROM users`, 1)
	assertCount(t, ctx, db, `
		SELECT count(*)
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 AND r.name = 'owner'
	`, 1, admin.ID)
	assertCount(t, ctx, db, `
		SELECT count(*)
		FROM audit_events
		WHERE actor_user_id IS NULL
		  AND action = $1
		  AND target_type = 'user'
		  AND target_id = $2
		  AND metadata->>'source' = 'bootstrap'
	`, 1, audit.ActionBootstrapAdminCreated, admin.ID)

	sessionNow := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	sessionService := auth.NewSessionService(auth.NewSQLSessionStore(db)).
		WithClock(func() time.Time { return sessionNow }).
		WithIDGenerator(func() (string, error) {
			return "30000000-0000-4000-8000-000000000020", nil
		}).
		WithTokenGenerator(func() (string, error) { return "raw-integration-session-token", nil })
	session, err := sessionService.Login(ctx, auth.LoginInput{
		Email:     strings.ToUpper(admin.Email),
		Password:  "correct horse battery staple",
		IPAddress: "192.0.2.10",
		UserAgent: "integration-browser",
	})
	if err != nil {
		t.Fatalf("login first admin: %v", err)
	}
	if session.Token != "raw-integration-session-token" || !session.ExpiresAt.Equal(sessionNow.Add(auth.DefaultSessionDuration)) {
		t.Fatalf("unexpected login session: %#v", session)
	}

	var storedTokenHash string
	var storedIPAddress string
	var storedUserAgent string
	if err := db.QueryRowContext(ctx, `
		SELECT token_hash, host(ip_address), user_agent
		FROM sessions
		WHERE user_id = $1
	`, admin.ID).Scan(&storedTokenHash, &storedIPAddress, &storedUserAgent); err != nil {
		t.Fatalf("read stored session: %v", err)
	}
	if storedTokenHash == "" || storedTokenHash == session.Token || storedIPAddress != "192.0.2.10" || storedUserAgent != "integration-browser" {
		t.Fatalf("stored session = hash:%q ip:%q user-agent:%q", storedTokenHash, storedIPAddress, storedUserAgent)
	}
	assertCount(t, ctx, db, `
		SELECT count(*) FROM audit_events
		WHERE action = $1
		  AND actor_user_id = $2
		  AND target_type = 'session'
		  AND target_id = $3
		  AND host(ip_address) = '192.0.2.10'
		  AND user_agent = 'integration-browser'
	`, 1, audit.ActionLoginSucceeded, admin.ID, "30000000-0000-4000-8000-000000000020")
	principal, err := sessionService.Validate(ctx, session.Token)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if principal.UserID != admin.ID || principal.Email != admin.Email || !principal.ExpiresAt.Equal(session.ExpiresAt) {
		t.Fatalf("validated principal = %#v", principal)
	}
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET expires_at = $1 WHERE user_id = $2`, sessionNow, admin.ID); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	if _, err := sessionService.Validate(ctx, session.Token); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("expired session validation error = %v, want %v", err, auth.ErrInvalidSession)
	}
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET expires_at = $1 WHERE user_id = $2`, session.ExpiresAt, admin.ID); err != nil {
		t.Fatalf("restore session expiry: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET is_active = FALSE WHERE id = $1`, admin.ID); err != nil {
		t.Fatalf("deactivate session user: %v", err)
	}
	if _, err := sessionService.Validate(ctx, session.Token); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("inactive user session validation error = %v, want %v", err, auth.ErrInvalidSession)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET is_active = TRUE WHERE id = $1`, admin.ID); err != nil {
		t.Fatalf("reactivate session user: %v", err)
	}

	for _, input := range []auth.LoginInput{
		{Email: "unknown@example.com", Password: "correct horse battery staple", IPAddress: "192.0.2.11", UserAgent: "integration-browser"},
		{Email: admin.Email, Password: "wrong password", IPAddress: "192.0.2.11", UserAgent: "integration-browser"},
	} {
		if _, err := sessionService.Login(ctx, input); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("invalid login error = %v, want %v", err, auth.ErrInvalidCredentials)
		}
	}
	assertCount(t, ctx, db, `
		SELECT count(*) FROM audit_events
		WHERE action = $1
		  AND actor_user_id IS NULL
		  AND metadata = '{}'::jsonb
	`, 2, audit.ActionLoginFailed)

	if err := sessionService.Logout(ctx, auth.LogoutInput{
		Token:     session.Token,
		IPAddress: "192.0.2.10",
		UserAgent: "integration-browser",
	}); err != nil {
		t.Fatalf("logout first admin: %v", err)
	}
	assertCount(t, ctx, db, `SELECT count(*) FROM sessions`, 0)
	assertCount(t, ctx, db, `
		SELECT count(*) FROM audit_events
		WHERE action = $1
		  AND actor_user_id = $2
		  AND host(ip_address) = '192.0.2.10'
		  AND user_agent = 'integration-browser'
	`, 1, audit.ActionLogoutSucceeded, admin.ID)
	if _, err := sessionService.Validate(ctx, session.Token); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("logged-out session validation error = %v, want %v", err, auth.ErrInvalidSession)
	}

	authorization := rbac.NewAuthorizer(rbac.NewSQLPermissionChecker(db))
	if err := authorization.Authorize(ctx, admin.ID, rbac.PermissionBillingManage); err != nil {
		t.Fatalf("authorize owner billing permission: %v", err)
	}
	if err := authorization.Authorize(ctx, admin.ID, "unknown:permission"); !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("unknown permission error = %v, want %v", err, rbac.ErrForbidden)
	}

	adminUserID := "30000000-0000-4000-8000-000000000030"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name)
		VALUES ($1, 'admin@example.com', $2, 'Admin')
	`, adminUserID, admin.PasswordHash); err != nil {
		t.Fatalf("insert admin user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = 'admin'
	`, adminUserID); err != nil {
		t.Fatalf("assign admin role: %v", err)
	}
	if err := authorization.Authorize(ctx, adminUserID, rbac.PermissionSettingsManage); err != nil {
		t.Fatalf("authorize admin settings permission: %v", err)
	}
	if err := authorization.Authorize(ctx, adminUserID, rbac.PermissionBillingManage); !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("admin billing permission error = %v, want %v", err, rbac.ErrForbidden)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET is_active = FALSE WHERE id = $1`, adminUserID); err != nil {
		t.Fatalf("deactivate admin user: %v", err)
	}
	if err := authorization.Authorize(ctx, adminUserID, rbac.PermissionSettingsManage); !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("inactive admin permission error = %v, want %v", err, rbac.ErrForbidden)
	}

	rolelessUserID := "30000000-0000-4000-8000-000000000031"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name)
		VALUES ($1, 'roleless@example.com', $2, 'Roleless')
	`, rolelessUserID, admin.PasswordHash); err != nil {
		t.Fatalf("insert roleless user: %v", err)
	}
	if err := authorization.Authorize(ctx, rolelessUserID, rbac.PermissionSettingsManage); !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("roleless user permission error = %v, want %v", err, rbac.ErrForbidden)
	}

	customerUserID := "30000000-0000-4000-8000-000000000032"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name)
		VALUES ($1, 'customer@example.com', $2, 'Customer')
	`, customerUserID, admin.PasswordHash); err != nil {
		t.Fatalf("insert customer user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = 'customer'
	`, customerUserID); err != nil {
		t.Fatalf("assign customer role: %v", err)
	}
	accountA := "40000000-0000-4000-8000-000000000001"
	accountB := "40000000-0000-4000-8000-000000000002"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_account_assignments (user_id, account_id)
		VALUES ($1, $2)
	`, customerUserID, accountA); err != nil {
		t.Fatalf("assign customer account: %v", err)
	}
	if err := authorization.AuthorizeAccount(ctx, rbac.Actor{UserID: customerUserID}, rbac.PermissionWebsitesManage, accountA); err != nil {
		t.Fatalf("authorize assigned customer account: %v", err)
	}
	if err := authorization.AuthorizeAccount(ctx, rbac.Actor{UserID: customerUserID}, rbac.PermissionWebsitesManage, accountB); !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("cross-account authorization error = %v, want %v", err, rbac.ErrForbidden)
	}
	if err := authorization.AuthorizeAccount(ctx, rbac.Actor{UserID: admin.ID}, rbac.PermissionWebsitesManage, accountB); err != nil {
		t.Fatalf("authorize owner global account access: %v", err)
	}

	httpSession, err := sessionService.Login(ctx, auth.LoginInput{
		Email:    admin.Email,
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("create HTTP integration session: %v", err)
	}
	auditStore := audit.NewSQLStore(db)
	app := server.New(server.Config{Sessions: sessionService, Authorization: authorization, AuditEvents: auditStore})
	auditRequest := httptest.NewRequest(http.MethodGet, "/api/audit-events", nil)
	auditRequest.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: httpSession.Token})
	auditResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(auditResponse, auditRequest)
	if auditResponse.Code != http.StatusOK || !strings.Contains(auditResponse.Body.String(), audit.ActionLoginSucceeded) || !strings.Contains(auditResponse.Body.String(), audit.ActionLoginFailed) {
		t.Fatalf("audit listing HTTP response = %d body=%q", auditResponse.Code, auditResponse.Body.String())
	}
	recentEvents, err := auditStore.ListRecent(ctx, audit.MaxRecentEvents)
	if err != nil {
		t.Fatalf("list recent audit events: %v", err)
	}
	if len(recentEvents) < 6 {
		t.Fatalf("recent audit event count = %d, want at least 6", len(recentEvents))
	}
	for index := 1; index < len(recentEvents); index++ {
		if recentEvents[index].CreatedAt.After(recentEvents[index-1].CreatedAt) {
			t.Fatalf("audit events are not newest first: %#v", recentEvents)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: httpSession.Token})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized HTTP status = %d body=%q", response.Code, response.Body.String())
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = $1`, admin.ID); err != nil {
		t.Fatalf("revoke owner role: %v", err)
	}
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"code":"forbidden"`) {
		t.Fatalf("revoked-role HTTP response = %d body=%q", response.Code, response.Body.String())
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = 'owner'
	`, admin.ID); err != nil {
		t.Fatalf("restore owner role: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET expires_at = $1 WHERE user_id = $2`, sessionNow, admin.ID); err != nil {
		t.Fatalf("expire HTTP integration session: %v", err)
	}
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"code":"unauthenticated"`) {
		t.Fatalf("expired-session HTTP response = %d body=%q", response.Code, response.Body.String())
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO audit_events (id, action, target_type, target_id, metadata, created_at)
		SELECT
			('50000000-0000-4000-8000-' || lpad(value::text, 12, '0'))::uuid,
			'auth.login.rejected',
			'authentication',
			'bulk-' || value,
			jsonb_build_object('attempt', value),
			'2099-01-01T00:00:00Z'::timestamptz
		FROM generate_series(1, 105) AS value
	`); err != nil {
		t.Fatalf("insert audit listing fixtures: %v", err)
	}
	limitedEvents, err := auditStore.ListRecent(ctx, audit.MaxRecentEvents)
	if err != nil {
		t.Fatalf("list capped audit events: %v", err)
	}
	if len(limitedEvents) != audit.MaxRecentEvents || limitedEvents[0].TargetID != "bulk-105" || limitedEvents[len(limitedEvents)-1].TargetID != "bulk-6" {
		t.Fatalf("capped audit event range = count:%d first:%q last:%q", len(limitedEvents), limitedEvents[0].TargetID, limitedEvents[len(limitedEvents)-1].TargetID)
	}
	if limitedEvents[0].Metadata["attempt"] != "105" {
		t.Fatalf("non-string audit metadata = %#v", limitedEvents[0].Metadata)
	}
	if _, err := db.ExecContext(ctx, `UPDATE audit_events SET action = 'auth.login.failed'`); err == nil {
		t.Fatal("expected append-only audit update to fail")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM audit_events`); err == nil {
		t.Fatal("expected append-only audit delete to fail")
	}

	rollbackSessionService := auth.NewSessionService(auth.NewSQLSessionStore(db)).
		WithClock(func() time.Time { return sessionNow }).
		WithIDGenerator(func() (string, error) { return "30000000-0000-4000-8000-000000000041", nil }).
		WithTokenGenerator(func() (string, error) { return "rollback-logout-token", nil })
	rollbackSession, err := rollbackSessionService.Login(ctx, auth.LoginInput{
		Email: admin.Email, Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("create rollback session fixture: %v", err)
	}
	var sessionsBeforeAuditFailure int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&sessionsBeforeAuditFailure); err != nil {
		t.Fatalf("count sessions before audit failure: %v", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE audit_events RENAME TO audit_events_unavailable`); err != nil {
		t.Fatalf("make audit table unavailable: %v", err)
	}
	failedLoginService := auth.NewSessionService(auth.NewSQLSessionStore(db)).
		WithClock(func() time.Time { return sessionNow }).
		WithIDGenerator(func() (string, error) { return "30000000-0000-4000-8000-000000000042", nil }).
		WithTokenGenerator(func() (string, error) { return "rollback-login-token", nil })
	if _, err := failedLoginService.Login(ctx, auth.LoginInput{
		Email: admin.Email, Password: "correct horse battery staple",
	}); err == nil {
		t.Fatal("expected login to fail when audit table is unavailable")
	}
	if err := rollbackSessionService.Logout(ctx, auth.LogoutInput{Token: rollbackSession.Token}); err == nil {
		t.Fatal("expected logout to fail when audit table is unavailable")
	}
	assertCount(t, ctx, db, `SELECT count(*) FROM sessions`, sessionsBeforeAuditFailure)
	if _, err := db.ExecContext(ctx, `ALTER TABLE audit_events_unavailable RENAME TO audit_events`); err != nil {
		t.Fatalf("restore audit table: %v", err)
	}
	if err := rollbackSessionService.Logout(ctx, auth.LogoutInput{Token: rollbackSession.Token}); err != nil {
		t.Fatalf("clean up rollback session: %v", err)
	}
}

func newTestBootstrapService(db *sql.DB, id string) auth.BootstrapService {
	return auth.NewBootstrapService(auth.NewSQLBootstrapStore(db)).
		WithIDGenerator(func() (string, error) { return id, nil }).
		WithPasswordHashParams(auth.PasswordHashParams{
			MemoryKB:    1024,
			Iterations:  1,
			Parallelism: 1,
			SaltLength:  8,
			KeyLength:   16,
		})
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func assertRolePermissions(t *testing.T, ctx context.Context, db rowQuerier, role string, want ...string) {
	t.Helper()

	var got string
	if err := db.QueryRowContext(ctx, `
		SELECT string_agg(p.code, ',' ORDER BY p.code)
		FROM role_permissions rp
		JOIN roles r ON r.id = rp.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE r.name = $1
	`, role).Scan(&got); err != nil {
		t.Fatalf("query %s permissions: %v", role, err)
	}
	if wantString := strings.Join(want, ","); got != wantString {
		t.Fatalf("%s permissions = %q, want %q", role, got, wantString)
	}
}

func assertCount(t *testing.T, ctx context.Context, db rowQuerier, query string, want int, args ...any) {
	t.Helper()

	var got int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}
