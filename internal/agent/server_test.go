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
	req.Header.Set("Content-Type", "application/json")

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
	req.Header.Set("Content-Type", "application/json")

	NewServer(ServerConfig{}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"action":"agent.health"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestActionEndpointRejectsInvalidTypedPayload(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/actions/agent.health", strings.NewReader(`{"payload":{"command":"shutdown"}}`))
	req.Header.Set("Content-Type", "application/json")

	NewServer(ServerConfig{}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_ACTION_PAYLOAD") || strings.Contains(rec.Body.String(), "shutdown") {
		t.Fatalf("response = %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestActionEndpointRejectsUnknownEnvelopeFieldsAndTrailingJSON(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		`{"payload":{},"command":"shutdown"}`,
		`{"payload":{}} {}`,
		`{"payload":{},"payload":{"command":"shutdown"}}`,
		`{"Payload":{}}`,
		`{"payload":null}`,
		`{}`,
		`null`,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/actions/agent.health", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		NewServer(ServerConfig{}).Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_JSON") {
			t.Fatalf("body %q response = %d %q", body, rec.Code, rec.Body.String())
		}
	}
}

func TestActionEndpointRejectsWrongContentTypeAndOversizedBody(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		body        string
		contentType string
	}{
		{name: "wrong content type", body: `{"payload":{}}`, contentType: "text/plain"},
		{name: "oversized body", body: `{"payload":{"value":"` + strings.Repeat("a", maxActionRequestBytes) + `"}}`, contentType: "application/json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/actions/agent.health", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)

			NewServer(ServerConfig{}).Handler().ServeHTTP(rec, req)

			wantStatus := http.StatusUnsupportedMediaType
			wantCode := "UNSUPPORTED_MEDIA_TYPE"
			if tc.name == "oversized body" {
				wantStatus = http.StatusRequestEntityTooLarge
				wantCode = "PAYLOAD_TOO_LARGE"
			}
			if rec.Code != wantStatus || !strings.Contains(rec.Body.String(), wantCode) {
				t.Fatalf("response = %d body=%q", rec.Code, rec.Body.String())
			}
		})
	}
}
