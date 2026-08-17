package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/motekar/motekar-panel/internal/agent"
	"github.com/motekar/motekar-panel/internal/server"
)

func TestPanelReadinessTracksLocalAgent(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "motekar-panel-agent-")
	if err != nil {
		t.Fatalf("create temp directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	listener, err := agent.ListenUnix(filepath.Join(dir, "agent.sock"))
	if err != nil {
		t.Fatalf("listen on agent socket: %v", err)
	}
	agentServer := &http.Server{Handler: agent.NewServer(agent.ServerConfig{}).Handler()}
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
	panel := server.New(server.Config{Ready: agentClient.Health})

	ready := httptest.NewRecorder()
	panel.Handler().ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("ready status with agent = %d, want %d; body=%s", ready.Code, http.StatusOK, ready.Body.String())
	}

	stopAgent()
	notReady := httptest.NewRecorder()
	panel.Handler().ServeHTTP(notReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status without agent = %d, want %d; body=%s", notReady.Code, http.StatusServiceUnavailable, notReady.Body.String())
	}
}
