package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/motekar/motekar-panel/internal/buildinfo"
	"github.com/motekar/motekar-panel/internal/rbac"
)

type ReadyFunc func(context.Context) error

type Config struct {
	Version       buildinfo.BuildInfo
	Ready         ReadyFunc
	Sessions      SessionAuthenticator
	Authorization PermissionAuthorizer
	SecureCookies bool
}

type Server struct {
	version       buildinfo.BuildInfo
	ready         ReadyFunc
	sessions      SessionAuthenticator
	authorization PermissionAuthorizer
	secureCookies bool
	loginLimiter  *loginLimiter
	sourceLimiter *loginLimiter
}

func New(cfg Config) *Server {
	if cfg.Ready == nil {
		cfg.Ready = func(context.Context) error { return nil }
	}
	return &Server{
		version:       cfg.Version,
		ready:         cfg.Ready,
		sessions:      cfg.Sessions,
		authorization: cfg.Authorization,
		secureCookies: cfg.SecureCookies,
		loginLimiter:  newLoginLimiter(loginAttemptLimit),
		sourceLimiter: newLoginLimiter(loginSourceAttemptLimit),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	home := s.RequirePermission(rbac.PermissionWebsitesManage, http.HandlerFunc(s.handleHome))
	mux.Handle("GET /", home)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /version", s.handleVersion)
	if s.sessions != nil {
		mux.HandleFunc("GET /login", s.handleLoginForm)
		mux.HandleFunc("POST /login", s.handleLogin)
		mux.HandleFunc("POST /logout", s.handleLogout)
	}
	return mux
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<!doctype html><title>Motekar Panel</title><h1>Motekar Panel</h1>"))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.ready(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.version)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
