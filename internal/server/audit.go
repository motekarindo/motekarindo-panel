package server

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"net/http"

	"github.com/motekar/motekar-panel/internal/audit"
)

const maxRecentAuditEvents = audit.MaxRecentEvents

var errAuditEventsUnavailable = errors.New("audit event reader is unavailable")

type AuditEventReader interface {
	ListRecent(ctx context.Context, limit int) ([]audit.Event, error)
}

type AuditRecorder interface {
	Record(ctx context.Context, event audit.Event) (audit.Event, error)
}

var auditEventsTemplate = template.Must(template.New("audit-events").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Audit events - Motekar Panel</title></head>
<body><main><h1>Recent audit events</h1><table><thead><tr><th>Time</th><th>Action</th><th>Actor</th><th>Target</th><th>IP address</th></tr></thead><tbody>
{{range .}}<tr><td>{{.CreatedAt}}</td><td>{{.Action}}</td><td>{{.ActorUserID}}</td><td>{{.TargetType}}/{{.TargetID}}</td><td>{{.IPAddress}}</td></tr>{{else}}<tr><td colspan="5">No audit events.</td></tr>{{end}}
</tbody></table></main></body></html>`))

func (s *Server) handleAuditEventsAPI(w http.ResponseWriter, r *http.Request) {
	s.setAuthResponseHeaders(w)
	events, err := s.listRecentAuditEvents(r.Context())
	if err != nil {
		writeAuthorizationError(w, http.StatusInternalServerError, "internal_error", "Unable to list audit events.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleAuditEventsHTML(w http.ResponseWriter, r *http.Request) {
	s.setAuthResponseHeaders(w)
	events, err := s.listRecentAuditEvents(r.Context())
	if err != nil {
		writeAuthorizationError(w, http.StatusInternalServerError, "internal_error", "Unable to list audit events.")
		return
	}
	var body bytes.Buffer
	if err := auditEventsTemplate.Execute(&body, events); err != nil {
		writeAuthorizationError(w, http.StatusInternalServerError, "internal_error", "Unable to render audit events.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = body.WriteTo(w)
}

func (s *Server) listRecentAuditEvents(ctx context.Context) ([]audit.Event, error) {
	if s.auditEvents == nil {
		return nil, errAuditEventsUnavailable
	}
	return s.auditEvents.ListRecent(ctx, maxRecentAuditEvents)
}
