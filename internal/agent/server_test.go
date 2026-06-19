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
