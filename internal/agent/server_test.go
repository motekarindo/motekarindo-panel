package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCapabilities(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/capabilities", nil)

	NewServer(ServerConfig{}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "agent.health") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestActionEndpointRejectsUnknownAction(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/actions/system.reboot", strings.NewReader(`{"payload":{}}`))

	NewServer(ServerConfig{}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if !strings.Contains(rec.Body.String(), "UNKNOWN_ACTION") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestActionEndpointExecutesHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/actions/agent.health", strings.NewReader(`{"payload":{}}`))

	NewServer(ServerConfig{}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"action":"agent.health"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
