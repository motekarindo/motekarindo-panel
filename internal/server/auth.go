package server

import (
	"context"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/auth"
)

const (
	SessionCookieName       = "motekar_session"
	maxLoginBodyBytes       = 8 << 10
	maxEmailBytes           = 320
	maxPasswordBytes        = 1024
	maxUserAgentRunes       = 512
	loginAttemptLimit       = 5
	loginSourceAttemptLimit = 50
	loginAuditAttemptLimit  = 10
	loginAttemptWindow      = 15 * time.Minute
	maxLoginLimiterEntries  = 10_000
)

type SessionAuthenticator interface {
	Login(ctx context.Context, input auth.LoginInput) (auth.LoginSession, error)
	Logout(ctx context.Context, input auth.LogoutInput) error
	Validate(ctx context.Context, token string) (auth.SessionPrincipal, error)
}

func (s *Server) handleLoginForm(w http.ResponseWriter, _ *http.Request) {
	s.setAuthResponseHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Sign in - Motekar Panel</title></head><body><main><h1>Sign in</h1><form method="post" action="/login"><label>Email <input type="email" name="email" autocomplete="username" required></label><label>Password <input type="password" name="password" autocomplete="current-password" required></label><button type="submit">Sign in</button></form></main></body></html>`))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	s.setAuthResponseHeaders(w)
	if !s.authRequestIsSameOrigin(r) {
		s.recordLoginRejection(r, "cross_origin")
		http.Error(w, "Forbidden.", http.StatusForbidden)
		return
	}
	ipAddress := requestIPAddress(r.RemoteAddr)

	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		s.recordLoginRejection(r, "malformed_request")
		http.Error(w, "Invalid email or password.", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.recordLoginRejection(r, "malformed_request")
		http.Error(w, "Invalid email or password.", http.StatusUnauthorized)
		return
	}
	email := r.PostForm.Get("email")
	password := r.PostForm.Get("password")
	if email == "" || password == "" || len(email) > maxEmailBytes || len(password) > maxPasswordBytes {
		s.recordLoginRejection(r, "invalid_credentials")
		http.Error(w, "Invalid email or password.", http.StatusUnauthorized)
		return
	}
	limiterKey := ipAddress + "\x00" + strings.ToLower(strings.TrimSpace(email))
	if !s.sourceLimiter.Allow(ipAddress) || !s.loginLimiter.Allow(limiterKey) {
		s.recordLoginRejection(r, "rate_limited")
		http.Error(w, "Too many login attempts.", http.StatusTooManyRequests)
		return
	}
	session, err := s.sessions.Login(r.Context(), auth.LoginInput{
		Email:     email,
		Password:  password,
		IPAddress: ipAddress,
		UserAgent: requestUserAgent(r),
	})
	if errors.Is(err, auth.ErrInvalidCredentials) {
		http.Error(w, "Invalid email or password.", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "Unable to sign in.", http.StatusInternalServerError)
		return
	}
	s.loginLimiter.Reset(limiterKey)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) recordLoginRejection(r *http.Request, reason string) {
	if s.auditRecorder == nil {
		return
	}
	ipAddress := requestIPAddress(r.RemoteAddr)
	if !s.auditLimiter.Allow(ipAddress) {
		return
	}
	_, err := s.auditRecorder.Record(r.Context(), audit.Event{
		Action:     audit.ActionLoginRejected,
		TargetType: "authentication",
		TargetID:   "login",
		IPAddress:  ipAddress,
		UserAgent:  requestUserAgent(r),
		Metadata:   map[string]string{"reason": reason},
	})
	if err != nil && s.auditError != nil {
		s.auditError(err)
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.setAuthResponseHeaders(w)
	if !s.authRequestIsSameOrigin(r) {
		http.Error(w, "Forbidden.", http.StatusForbidden)
		return
	}
	var token string
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		token = cookie.Value
	}
	if err := s.sessions.Logout(r.Context(), auth.LogoutInput{
		Token:     token,
		IPAddress: requestIPAddress(r.RemoteAddr),
		UserAgent: requestUserAgent(r),
	}); err != nil {
		http.Error(w, "Unable to sign out.", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func requestUserAgent(r *http.Request) string {
	userAgentRunes := []rune(strings.ToValidUTF8(r.UserAgent(), ""))
	if len(userAgentRunes) > maxUserAgentRunes {
		userAgentRunes = userAgentRunes[:maxUserAgentRunes]
	}
	return string(userAgentRunes)
}

func (s *Server) authRequestIsSameOrigin(r *http.Request) bool {
	fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	if fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
		return false
	}
	source := strings.TrimSpace(r.Header.Get("Origin"))
	if source == "" {
		source = strings.TrimSpace(r.Header.Get("Referer"))
	}
	if source == "" {
		return false
	}
	parsed, err := url.Parse(source)
	if err != nil || parsed.Host != r.Host {
		return false
	}
	expectedScheme := "http"
	if s.secureCookies {
		expectedScheme = "https"
	}
	return parsed.Scheme == expectedScheme
}

func (s *Server) setAuthResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	if s.secureCookies {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
}

func requestIPAddress(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if net.ParseIP(host) == nil {
		return ""
	}
	return host
}

type loginAttempt struct {
	count       int
	windowStart time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
	now      func() time.Time
	limit    int
}

func newLoginLimiter(limit int) *loginLimiter {
	return &loginLimiter{
		attempts: make(map[string]loginAttempt),
		now:      time.Now,
		limit:    limit,
	}
}

func (l *loginLimiter) Allow(key string) bool {
	if strings.TrimSpace(key) == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	attempt, exists := l.attempts[key]
	if !exists && len(l.attempts) >= maxLoginLimiterEntries {
		oldestKey := ""
		var oldest time.Time
		for attemptKey, existing := range l.attempts {
			if now.Sub(existing.windowStart) >= loginAttemptWindow {
				delete(l.attempts, attemptKey)
				continue
			}
			if oldestKey == "" || existing.windowStart.Before(oldest) {
				oldestKey = attemptKey
				oldest = existing.windowStart
			}
		}
		if len(l.attempts) >= maxLoginLimiterEntries && oldestKey != "" {
			delete(l.attempts, oldestKey)
		}
	}
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) >= loginAttemptWindow {
		attempt = loginAttempt{windowStart: now}
	}
	if attempt.count >= l.limit {
		return false
	}
	attempt.count++
	l.attempts[key] = attempt
	return true
}

func (l *loginLimiter) Reset(key string) {
	if strings.TrimSpace(key) == "" {
		key = "unknown"
	}
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}
