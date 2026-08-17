package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/rbac"
)

func TestInventoryShowsSystemFactsAndEscapesValues(t *testing.T) {
	t.Parallel()

	fetcher := &fakeInventoryFetcher{inventory: Inventory{
		AgentStatus:    "online",
		OS:             "Ubuntu 24.04 LTS ubuntu 24.04",
		Kernel:         "6.8.0-45-generic",
		CPUCores:       4,
		RAMTotalMB:     15625,
		RAMAvailableMB: 7812,
		SwapMB:         1953,
		DiskFreeGB:     42,
		Load1:          0.52,
		Load5:          0.35,
		Load15:         0.25,
		UptimeSeconds:  4321,
		IPAddresses:    []string{"192.0.2.10", "2001:db8::10"},
		Services:       []string{"nginx", "postgresql"},
		WebServer:      "nginx",
		HasSystemd:     true,
	}}
	app, authorizer := newInventoryTestServer(fetcher)
	request := authorizedInventoryRequest(http.MethodGet, "/inventory")
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("response = %d content-type=%q", response.Code, response.Header().Get("Content-Type"))
	}
	for _, want := range []string{
		"online", "Ubuntu 24.04 LTS ubuntu 24.04", "6.8.0-45-generic", "4", "15625 MB total / 7812 MB free",
		"1953 MB", "42 GB", "192.0.2.10", "nginx", "postgresql", "Server overview",
	} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body does not contain %q", want)
		}
	}
	if authorizer.permission != rbac.PermissionSettingsManage {
		t.Fatalf("permission = %q, want %q", authorizer.permission, rbac.PermissionSettingsManage)
	}
}

func TestInventoryShowsMissingDataAsNotAvailable(t *testing.T) {
	t.Parallel()

	fetcher := &fakeInventoryFetcher{inventory: Inventory{AgentStatus: "online"}}
	app, _ := newInventoryTestServer(fetcher)
	request := authorizedInventoryRequest(http.MethodGet, "/inventory")
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("response = %d", response.Code)
	}
	for _, want := range []string{"Not available", "Not selected"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body does not contain %q", want)
		}
	}
}

func TestInventoryShowsUnavailableAgentMessage(t *testing.T) {
	t.Parallel()

	fetcher := &fakeInventoryFetcher{inventory: Inventory{
		AgentStatus:  "unavailable",
		AgentMessage: "The local agent rejected the inventory request (HTTP 404, UNKNOWN_ACTION).",
	}}
	app, _ := newInventoryTestServer(fetcher)
	request := authorizedInventoryRequest(http.MethodGet, "/inventory")
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("response = %d", response.Code)
	}
	for _, want := range []string{"unavailable", "Agent unavailable", "HTTP 404"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body does not contain %q", want)
		}
	}
}

func TestInventoryRequiresPermission(t *testing.T) {
	t.Parallel()

	authorizer := &fakePermissionAuthorizer{err: rbac.ErrForbidden}
	app := New(Config{
		Sessions:      &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id"}},
		Authorization: authorizer,
		Inventory:     &fakeInventoryFetcher{},
	})
	request := authorizedInventoryRequest(http.MethodGet, "/inventory")
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || authorizer.permission != rbac.PermissionSettingsManage {
		t.Fatalf("response = %d permission=%q", response.Code, authorizer.permission)
	}
}

func TestInventoryHandlerFailsOnFetcherError(t *testing.T) {
	t.Parallel()

	app, _ := newInventoryTestServer(&fakeInventoryFetcher{err: errors.New("inventory failure")})
	request := authorizedInventoryRequest(http.MethodGet, "/inventory")
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("response = %d", response.Code)
	}
}

func newInventoryTestServer(fetcher InventoryFetcher) (*Server, *fakePermissionAuthorizer) {
	authorizer := &fakePermissionAuthorizer{}
	return New(Config{
		Sessions:      &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id"}},
		Authorization: authorizer,
		Inventory:     fetcher,
	}), authorizer
}

func authorizedInventoryRequest(method, target string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	return request
}

type fakeInventoryFetcher struct {
	inventory Inventory
	err       error
}

func (f *fakeInventoryFetcher) Fetch(context.Context) (Inventory, error) {
	return f.inventory, f.err
}
