package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/motekar/motekar-panel/internal/agent"
	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/server"
)

func TestInventoryEndpointUsesLiveAgentSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "motekar-panel-inventory-")
	if err != nil {
		t.Fatalf("create temp directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	writeIntegrationFile(t, filepath.Join(dir, "os-release"), "ID=ubuntu\nVERSION_ID=\"24.04\"\nNAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n")
	writeIntegrationFile(t, filepath.Join(dir, "osrelease"), "6.8.0-45-generic\n")
	writeIntegrationFile(t, filepath.Join(dir, "meminfo"), "MemTotal:       16000000 kB\nMemAvailable:    8000000 kB\nSwapTotal:       2000000 kB\n")
	writeIntegrationFile(t, filepath.Join(dir, "loadavg"), "0.52 0.35 0.25 1/100 1234\n")
	writeIntegrationFile(t, filepath.Join(dir, "uptime"), "4321.50 12345.67\n")

	listener, err := agent.ListenUnix(filepath.Join(dir, "agent.sock"))
	if err != nil {
		t.Fatalf("listen on agent socket: %v", err)
	}
	agentServer := &http.Server{Handler: agent.NewServer(agent.ServerConfig{
		Registry: agent.NewInventoryRegistry(agent.InventoryCollector{
			OSReleasePath: filepath.Join(dir, "os-release"),
			KernelPath:    filepath.Join(dir, "osrelease"),
			MeminfoPath:   filepath.Join(dir, "meminfo"),
			LoadavgPath:   filepath.Join(dir, "loadavg"),
			UptimePath:    filepath.Join(dir, "uptime"),
			SystemdPath:   filepath.Join(dir, "no-systemd"),
			Proc1CommPath: filepath.Join(dir, "no-proc1"),
			DiskPath:      dir,
			IPAddresses:   func() ([]string, error) { return []string{"192.0.2.10"}, nil },
		}),
	}).Handler()}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- agentServer.Serve(listener)
	}()
	stopped := false
	stopAgent := func() {
		if stopped {
			return
		}
		stopped = true
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = agentServer.Shutdown(ctx)
		<-serveDone
	}
	t.Cleanup(stopAgent)

	agentClient := agent.NewUnixClient(listener.Addr().String(), 250*time.Millisecond)
	app := server.New(server.Config{
		Sessions:      &integrationSessionAuthenticator{},
		Authorization: &integrationPermissionAuthorizer{},
		Inventory:     agentInventoryFetcher{client: agentClient},
	})

	online := httptest.NewRecorder()
	app.Handler().ServeHTTP(online, integrationInventoryRequest())
	if online.Code != http.StatusOK {
		t.Fatalf("inventory with agent = %d, want %d; body=%s", online.Code, http.StatusOK, online.Body.String())
	}
	t.Logf("online body: %s", online.Body.String())
	for _, want := range []string{"online", "Server overview", "Ubuntu 24.04 LTS", "6.8.0-45-generic", "192.0.2.10"} {
		if !strings.Contains(online.Body.String(), want) {
			t.Fatalf("inventory body does not contain %q", want)
		}
	}
	if strings.Contains(online.Body.String(), "Agent unavailable") {
		t.Fatalf("inventory body reports unavailable while agent is online: %s", online.Body.String())
	}

	stopAgent()
	offline := httptest.NewRecorder()
	app.Handler().ServeHTTP(offline, integrationInventoryRequest())
	if offline.Code != http.StatusOK {
		t.Fatalf("inventory without agent = %d, want %d; body=%s", offline.Code, http.StatusOK, offline.Body.String())
	}
	if !strings.Contains(offline.Body.String(), "Agent unavailable") {
		t.Fatalf("inventory body does not report unavailable: %s", offline.Body.String())
	}
}

func integrationInventoryRequest() *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/inventory", nil)
	request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: "raw-token"})
	return request
}

func writeIntegrationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type integrationSessionAuthenticator struct{}

func (integrationSessionAuthenticator) Login(context.Context, auth.LoginInput) (auth.LoginSession, error) {
	return auth.LoginSession{}, nil
}

func (integrationSessionAuthenticator) Logout(context.Context, auth.LogoutInput) error {
	return nil
}

func (integrationSessionAuthenticator) Validate(context.Context, string) (auth.SessionPrincipal, error) {
	return auth.SessionPrincipal{UserID: "user-id"}, nil
}

type integrationPermissionAuthorizer struct{}

func (integrationPermissionAuthorizer) Authorize(_ context.Context, _, _ string) error {
	return nil
}
