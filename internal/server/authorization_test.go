package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/rbac"
)

func TestRequirePermissionAllowsAuthorizedSession(t *testing.T) {
	t.Parallel()

	sessions := &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id", Email: "owner@example.com"}}
	authorizer := &fakePermissionAuthorizer{}
	app := New(Config{Sessions: sessions, Authorization: authorizer})
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		principal, ok := SessionPrincipalFromContext(r.Context())
		if !ok || principal.UserID != "user-id" {
			t.Fatalf("context principal = %#v, present = %v", principal, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	rec := httptest.NewRecorder()

	app.RequirePermission(rbac.PermissionUsersManage, next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent || !nextCalled {
		t.Fatalf("response = %d next called = %v", rec.Code, nextCalled)
	}
	if sessions.validatedToken != "raw-token" || authorizer.userID != "user-id" || authorizer.permission != rbac.PermissionUsersManage {
		t.Fatalf("validation = token:%q user:%q permission:%q", sessions.validatedToken, authorizer.userID, authorizer.permission)
	}
}

func TestHandlerProtectsHomeWhenAuthorizationIsConfigured(t *testing.T) {
	t.Parallel()

	app := New(Config{
		Sessions:      &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id"}},
		Authorization: &fakePermissionAuthorizer{},
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"code":"unauthenticated"`) {
		t.Fatalf("response = %d body=%q", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Motekar Panel") {
		t.Fatalf("authorized response = %d body=%q", response.Code, response.Body.String())
	}
}

func TestHandlerFailsClosedWhenAuthorizationIsNotConfigured(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	New(Config{}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"internal_error"`) {
		t.Fatalf("response = %d body=%q", response.Code, response.Body.String())
	}
}

func TestRequirePermissionReturnsStructuredAuthenticationError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		sessions *fakeSessionAuthenticator
		cookie   string
	}{
		{name: "missing cookie", sessions: &fakeSessionAuthenticator{}},
		{name: "invalid session", sessions: &fakeSessionAuthenticator{validateErr: auth.ErrInvalidSession}, cookie: "invalid-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := New(Config{Sessions: tc.sessions, Authorization: &fakePermissionAuthorizer{}})
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tc.cookie})
			}
			rec := httptest.NewRecorder()

			app.RequirePermission(rbac.PermissionUsersManage, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("protected handler must not run")
			})).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"code":"unauthenticated"`) {
				t.Fatalf("response = %d body=%q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRequirePermissionReturnsStructuredForbiddenError(t *testing.T) {
	t.Parallel()

	sessions := &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id"}}
	authorizer := &fakePermissionAuthorizer{err: rbac.ErrForbidden}
	app := New(Config{Sessions: sessions, Authorization: authorizer})
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	rec := httptest.NewRecorder()

	app.RequirePermission(rbac.PermissionUsersManage, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler must not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"forbidden"`) {
		t.Fatalf("response = %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRequirePermissionHidesInternalAuthorizationErrors(t *testing.T) {
	t.Parallel()

	sessions := &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id"}}
	authorizer := &fakePermissionAuthorizer{err: errors.New("database password leaked")}
	app := New(Config{Sessions: sessions, Authorization: authorizer})
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	rec := httptest.NewRecorder()

	app.RequirePermission(rbac.PermissionUsersManage, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler must not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"code":"internal_error"`) || strings.Contains(rec.Body.String(), "database password") {
		t.Fatalf("response = %d body=%q", rec.Code, rec.Body.String())
	}
}

type fakePermissionAuthorizer struct {
	userID     string
	permission string
	err        error
}

func (f *fakePermissionAuthorizer) Authorize(_ context.Context, userID, permission string) error {
	f.userID = userID
	f.permission = permission
	return f.err
}
