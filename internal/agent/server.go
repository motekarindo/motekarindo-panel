package agent

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/motekar/motekar-panel/internal/buildinfo"
)

type ServerConfig struct {
	Version buildinfo.BuildInfo
}

type Server struct {
	version buildinfo.BuildInfo
	actions *Registry
}

func NewServer(cfg ServerConfig) *Server {
	return &Server{
		version: cfg.Version,
		actions: DefaultRegistry(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /capabilities", s.handleCapabilities)
	mux.HandleFunc("GET /version", s.handleVersion)
	mux.HandleFunc("POST /actions/{name}", s.handleAction)
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

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]string{
				"code":    "INVALID_JSON",
				"message": "Request body must be valid JSON.",
			},
		})
		return
	}

	result, err := s.actions.Execute(r.Context(), name, body.Payload)
	if err != nil {
		status := http.StatusBadRequest
		code := "ACTION_ERROR"
		if errors.Is(err, ErrUnknownAction) {
			status = http.StatusNotFound
			code = "UNKNOWN_ACTION"
		}
		writeJSON(w, status, map[string]any{
			"error": map[string]string{
				"code":    code,
				"message": err.Error(),
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
