package agent

import (
	"encoding/json"
	"net/http"

	"github.com/motekar/motekar-panel/internal/buildinfo"
)

type ServerConfig struct {
	Version buildinfo.BuildInfo
}

type Server struct {
	version buildinfo.BuildInfo
}

func NewServer(cfg ServerConfig) *Server {
	return &Server{version: cfg.Version}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /capabilities", s.handleCapabilities)
	mux.HandleFunc("GET /version", s.handleVersion)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, DefaultCapabilities())
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.version)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
