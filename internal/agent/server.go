package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/motekar/motekar-panel/internal/buildinfo"
)

const maxActionRequestBytes = 64 << 10

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
	r.Body = http.MaxBytesReader(w, r.Body, maxActionRequestBytes)
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeActionRequestError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json.")
		return
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeActionRequestError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body exceeds the 64 KiB limit.")
			return
		}
		writeInvalidJSON(w)
		return
	}
	rawBody = bytes.TrimSpace(rawBody)
	if len(rawBody) == 0 || rawBody[0] != '{' || rejectDuplicateJSONKeys(rawBody) != nil {
		writeInvalidJSON(w)
		return
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &envelope); err != nil || len(envelope) != 1 {
		writeInvalidJSON(w)
		return
	}
	payload, exists := envelope["payload"]
	payload = bytes.TrimSpace(payload)
	if !exists || len(payload) == 0 || bytes.Equal(payload, []byte("null")) || payload[0] != '{' {
		writeInvalidJSON(w)
		return
	}

	result, err := s.actions.Execute(r.Context(), name, payload)
	if err != nil {
		status := http.StatusInternalServerError
		code := "ACTION_ERROR"
		message := "Action execution failed."
		if errors.Is(err, ErrUnknownAction) {
			status = http.StatusNotFound
			code = "UNKNOWN_ACTION"
			message = "Action is not allowlisted."
		} else if errors.Is(err, ErrInvalidPayload) {
			status = http.StatusBadRequest
			code = "INVALID_ACTION_PAYLOAD"
			message = "Payload does not match the action schema."
		}
		writeJSON(w, status, map[string]any{
			"error": map[string]string{
				"code":    code,
				"message": message,
			},
		})
		return
	}

	writeEncodedJSON(w, http.StatusOK, result.encodedJSON)
}

func writeInvalidJSON(w http.ResponseWriter) {
	writeActionRequestError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must contain one JSON object with a non-null payload object.")
}

func writeActionRequestError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		encoded = []byte(`{"error":{"code":"ENCODING_ERROR","message":"Unable to encode response."}}`)
	}
	writeEncodedJSON(w, status, encoded)
}

func writeEncodedJSON(w http.ResponseWriter, status int, encoded []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}
