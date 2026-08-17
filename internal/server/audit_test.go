package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/rbac"
)

func TestAuditEventsAPIListsRecentEvents(t *testing.T) {
	t.Parallel()

	reader := &fakeAuditEventReader{events: []audit.Event{{
		ID:          "event-id",
		ActorUserID: "user-id",
		Action:      audit.ActionLoginSucceeded,
		TargetType:  "session",
		TargetID:    "session-id",
		IPAddress:   "192.0.2.10",
		UserAgent:   "test-browser",
		Metadata:    map[string]string{"outcome": "success"},
		CreatedAt:   time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
	}}}
	authorizer := &fakePermissionAuthorizer{}
	app := New(Config{
		Sessions:      &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id"}},
		Authorization: authorizer,
		AuditEvents:   reader,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/audit-events", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("response = %d content-type=%q", response.Code, response.Header().Get("Content-Type"))
	}
	for _, want := range []string{`"events"`, `"action":"auth.login.succeeded"`, `"actorUserId":"user-id"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body %q does not contain %q", response.Body.String(), want)
		}
	}
	if reader.limit != maxRecentAuditEvents || authorizer.permission != rbac.PermissionAuditRead {
		t.Fatalf("list limit = %d permission = %q", reader.limit, authorizer.permission)
	}
}

func TestAuditEventsHTMLViewEscapesEventValues(t *testing.T) {
	t.Parallel()

	reader := &fakeAuditEventReader{events: []audit.Event{{
		ID:         "event-id",
		Action:     "<script>alert(1)</script>",
		TargetType: "user",
		TargetID:   "user-id",
		CreatedAt:  time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
	}}}
	app := New(Config{
		Sessions:      &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id"}},
		Authorization: &fakePermissionAuthorizer{},
		AuditEvents:   reader,
	})
	request := httptest.NewRequest(http.MethodGet, "/audit-events", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "&lt;script&gt;alert(1)&lt;/script&gt;") || strings.Contains(response.Body.String(), "<script>alert(1)</script>") {
		t.Fatalf("response = %d body=%q", response.Code, response.Body.String())
	}
}

func TestAuditEventsAPIHidesReaderErrors(t *testing.T) {
	t.Parallel()

	app := New(Config{
		Sessions:      &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id"}},
		Authorization: &fakePermissionAuthorizer{},
		AuditEvents:   &fakeAuditEventReader{err: errors.New("database password leaked")},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/audit-events", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"internal_error"`) || strings.Contains(response.Body.String(), "database password") {
		t.Fatalf("response = %d body=%q", response.Code, response.Body.String())
	}
}

type fakeAuditEventReader struct {
	events []audit.Event
	err    error
	limit  int
}

func (f *fakeAuditEventReader) ListRecent(_ context.Context, limit int) ([]audit.Event, error) {
	f.limit = limit
	return f.events, f.err
}
