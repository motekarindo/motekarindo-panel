package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/auth"
)

func TestLoginSetsSecureSessionCookie(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC()
	sessions := &fakeSessionAuthenticator{
		loginSession: auth.LoginSession{Token: "raw-token", ExpiresAt: expiresAt},
	}
	app := New(Config{Sessions: sessions, SecureCookies: true})
	form := url.Values{"email": {"owner@example.com"}, "password": {"correct horse battery staple"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "test-browser")
	setSameOrigin(req, true)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("response = %d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if sessions.loginInput.Email != "owner@example.com" || sessions.loginInput.IPAddress != "127.0.0.1" || sessions.loginInput.UserAgent != "test-browser" {
		t.Fatalf("login input = %#v", sessions.loginInput)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName || cookie.Value != "raw-token" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("session cookie = %#v", cookie)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatalf("security headers = %#v", rec.Header())
	}
}

func TestLoginReturnsGenericInvalidCredentials(t *testing.T) {
	sessions := &fakeSessionAuthenticator{loginErr: auth.ErrInvalidCredentials}
	app := New(Config{Sessions: sessions})
	form := url.Values{"email": {"unknown@example.com"}, "password": {"wrong password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setSameOrigin(req, false)
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || strings.TrimSpace(rec.Body.String()) != "Invalid email or password." {
		t.Fatalf("response = %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestLoginRejectsQueryStringCredentials(t *testing.T) {
	t.Parallel()

	sessions := &fakeSessionAuthenticator{}
	app := New(Config{Sessions: sessions})
	req := httptest.NewRequest(http.MethodPost, "/login?email=owner@example.com&password=secret", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setSameOrigin(req, false)
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || sessions.loginInput.Email != "" {
		t.Fatalf("response = %d login input = %#v", rec.Code, sessions.loginInput)
	}
}

func TestLoginRateLimit(t *testing.T) {
	sessions := &fakeSessionAuthenticator{loginErr: auth.ErrInvalidCredentials}
	app := New(Config{Sessions: sessions})
	for attempt := 1; attempt <= loginAttemptLimit; attempt++ {
		form := url.Values{"email": {"owner@example.com"}, "password": {"wrong password"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		setSameOrigin(req, false)
		req.RemoteAddr = "192.0.2.1:12345"
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d", attempt, rec.Code)
		}
	}

	otherForm := url.Values{"email": {"other@example.com"}, "password": {"wrong password"}}
	otherRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(otherForm.Encode()))
	otherRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setSameOrigin(otherRequest, false)
	otherRequest.RemoteAddr = "192.0.2.1:12345"
	otherResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(otherResponse, otherRequest)
	if otherResponse.Code != http.StatusUnauthorized {
		t.Fatalf("other account status = %d", otherResponse.Code)
	}

	blockedForm := url.Values{"email": {"owner@example.com"}, "password": {"wrong password"}}
	blockedRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(blockedForm.Encode()))
	blockedRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setSameOrigin(blockedRequest, false)
	blockedRequest.RemoteAddr = "192.0.2.1:12345"
	blockedResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(blockedResponse, blockedRequest)
	if blockedResponse.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited status = %d", blockedResponse.Code)
	}
}

func TestAuthRejectsCrossOriginRequests(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		origin    string
		fetchSite string
	}{
		{name: "hostile origin", origin: "https://attacker.example"},
		{name: "cross site metadata", origin: "http://example.com", fetchSite: "cross-site"},
		{name: "same site metadata", origin: "http://example.com", fetchSite: "same-site"},
		{name: "missing provenance"},
	}
	for _, tc := range cases {
		for _, target := range []string{"/login", "/logout"} {
			t.Run(tc.name+" "+target, func(t *testing.T) {
				sessions := &fakeSessionAuthenticator{}
				app := New(Config{Sessions: sessions})
				req := httptest.NewRequest(http.MethodPost, target, nil)
				if tc.origin != "" {
					req.Header.Set("Origin", tc.origin)
				}
				if tc.fetchSite != "" {
					req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
				}
				rec := httptest.NewRecorder()

				app.Handler().ServeHTTP(rec, req)

				if rec.Code != http.StatusForbidden || sessions.loginInput.Email != "" || sessions.logoutToken != "" {
					t.Fatalf("response = %d login=%#v logout=%q", rec.Code, sessions.loginInput, sessions.logoutToken)
				}
			})
		}
	}
}

func TestLoginRejectionAuditIsBoundedAndCredentialFree(t *testing.T) {
	t.Parallel()

	recorder := &fakeAuditRecorder{err: errors.New("audit unavailable")}
	auditErrors := 0
	app := New(Config{
		Sessions:      &fakeSessionAuthenticator{},
		AuditRecorder: recorder,
		AuditError:    func(error) { auditErrors++ },
	})
	for range loginAuditAttemptLimit + 5 {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=do-not-record"))
		req.Header.Set("Origin", "https://attacker.example")
		req.Header.Set("User-Agent", "test-browser")
		req.RemoteAddr = "192.0.2.50:12345"
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d", rec.Code)
		}
	}
	if len(recorder.events) != loginAuditAttemptLimit {
		t.Fatalf("audit event count = %d, want %d", len(recorder.events), loginAuditAttemptLimit)
	}
	if auditErrors != loginAuditAttemptLimit {
		t.Fatalf("reported audit error count = %d, want %d", auditErrors, loginAuditAttemptLimit)
	}
	for _, event := range recorder.events {
		if event.Action != audit.ActionLoginRejected || event.Metadata["reason"] != "cross_origin" || event.IPAddress != "192.0.2.50" {
			t.Fatalf("audit event = %#v", event)
		}
		if strings.Contains(fmt.Sprint(event), "do-not-record") {
			t.Fatalf("audit event contains credential: %#v", event)
		}
	}
}

func TestLoginSourceRateLimit(t *testing.T) {
	t.Parallel()

	sessions := &fakeSessionAuthenticator{loginErr: auth.ErrInvalidCredentials}
	app := New(Config{Sessions: sessions})
	for attempt := 1; attempt <= loginSourceAttemptLimit+1; attempt++ {
		form := url.Values{
			"email":    {fmt.Sprintf("user-%d@example.com", attempt)},
			"password": {"wrong password"},
		}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		setSameOrigin(req, false)
		req.RemoteAddr = "192.0.2.2:12345"
		rec := httptest.NewRecorder()

		app.Handler().ServeHTTP(rec, req)

		if attempt <= loginSourceAttemptLimit && rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d", attempt, rec.Code)
		}
		if attempt > loginSourceAttemptLimit && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("source-limited status = %d", rec.Code)
		}
	}
}

func TestLogoutInvalidatesSessionAndClearsCookie(t *testing.T) {
	sessions := &fakeSessionAuthenticator{}
	app := New(Config{Sessions: sessions, SecureCookies: true})
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	setSameOrigin(req, true)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("response = %d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if sessions.logoutToken != "raw-token" {
		t.Fatalf("logout token = %q", sessions.logoutToken)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("logout cookie = %#v", cookies)
	}
}

func TestLogoutKeepsCookieWhenInvalidationFails(t *testing.T) {
	sessions := &fakeSessionAuthenticator{logoutErr: errors.New("database unavailable")}
	app := New(Config{Sessions: sessions})
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	setSameOrigin(req, false)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("response = %d cookies=%#v", rec.Code, rec.Result().Cookies())
	}
}

type fakeSessionAuthenticator struct {
	loginInput     auth.LoginInput
	loginSession   auth.LoginSession
	loginErr       error
	logoutToken    string
	logoutErr      error
	principal      auth.SessionPrincipal
	validatedToken string
	validateErr    error
}

func (f *fakeSessionAuthenticator) Login(_ context.Context, input auth.LoginInput) (auth.LoginSession, error) {
	f.loginInput = input
	return f.loginSession, f.loginErr
}

func (f *fakeSessionAuthenticator) Logout(_ context.Context, input auth.LogoutInput) error {
	f.logoutToken = input.Token
	return f.logoutErr
}

func (f *fakeSessionAuthenticator) Validate(_ context.Context, token string) (auth.SessionPrincipal, error) {
	f.validatedToken = token
	return f.principal, f.validateErr
}

func setSameOrigin(req *http.Request, secure bool) {
	scheme := "http"
	if secure {
		scheme = "https"
	}
	req.Header.Set("Origin", scheme+"://"+req.Host)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}

type fakeAuditRecorder struct {
	events []audit.Event
	err    error
}

func (f *fakeAuditRecorder) Record(_ context.Context, event audit.Event) (audit.Event, error) {
	f.events = append(f.events, event)
	return event, f.err
}
