package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnixTransportServesHealthAndCapabilities(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "run", "agent.sock")
	listener, err := ListenUnix(socketPath)
	if err != nil {
		t.Fatalf("listen on Unix socket: %v", err)
	}

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat Unix socket: %v", err)
	}
	if info.Mode().Perm() != 0o660 {
		t.Fatalf("socket permissions = %o, want 660", info.Mode().Perm())
	}

	agentServer := NewServer(ServerConfig{})
	agentServer.actions.MustRegister(MustDefineAction("idempotency.inspect", validateEmptyPayload, func(ctx context.Context, _ EmptyPayload) (ActionResult, error) {
		return ActionResult{Status: "ok", Data: map[string]any{"idempotency_key": IdempotencyKey(ctx)}}, nil
	}))
	agentServer.actions.MustRegister(MustDefineAction("payload.inspect", func(largeActionPayload) error { return nil }, func(_ context.Context, payload largeActionPayload) (ActionResult, error) {
		return ActionResult{Status: "ok", Data: map[string]any{"length": len(payload.Value)}}, nil
	}))
	httpServer := &http.Server{Handler: agentServer.Handler()}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- httpServer.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
		<-serveDone
	})

	client := NewUnixClient(socketPath, time.Second)
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("agent health: %v", err)
	}
	capabilities, err := client.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("agent capabilities: %v", err)
	}
	if len(capabilities.Actions) != 3 || capabilities.Actions[0] != "agent.capabilities" || capabilities.Actions[1] != "agent.health" || capabilities.Actions[2] != "server.inventory" {
		t.Fatalf("capabilities = %#v", capabilities.Actions)
	}
	result, err := client.Execute(context.Background(), "agent.health", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute agent action: %v", err)
	}
	if result.Action != "agent.health" || result.Status != "ok" {
		t.Fatalf("action result = %#v", result)
	}
	result, err = client.ExecuteJob(context.Background(), "idempotency.inspect", json.RawMessage(`{}`), "job-key")
	if err != nil {
		t.Fatalf("execute idempotent agent action: %v", err)
	}
	if result.Data["idempotency_key"] != "job-key" {
		t.Fatalf("idempotency result = %#v", result.Data)
	}
	valueLength := maxActionPayloadBytes - len(`{"value":""}`)
	largePayload := json.RawMessage(`{"value":"` + strings.Repeat("<", valueLength) + `"}`)
	result, err = client.ExecuteJob(context.Background(), "payload.inspect", largePayload, "large-job")
	if err != nil {
		t.Fatalf("execute maximum-size payload: %v", err)
	}
	if result.Data["length"] != float64(valueLength) {
		t.Fatalf("maximum payload result = %#v", result.Data)
	}
}

type largeActionPayload struct {
	Value string `json:"value"`
}

func TestListenUnixRefusesNonSocketPath(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "agent.sock")
	if err := os.WriteFile(path, []byte("do not replace"), 0o600); err != nil {
		t.Fatalf("write sentinel file: %v", err)
	}

	if _, err := ListenUnix(path); err == nil {
		t.Fatal("expected non-socket path rejection")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sentinel file: %v", err)
	}
	if string(content) != "do not replace" {
		t.Fatalf("sentinel file was modified: %q", content)
	}
}

func TestListenUnixLocksActiveSocketUntilClose(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "agent.sock")
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("first listen: %v", err)
	}

	if _, err := ListenUnix(path); !errors.Is(err, ErrSocketInUse) {
		t.Fatalf("second listen error = %v, want %v", err, ErrSocketInUse)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close first listener: %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket still exists after close: %v", err)
	}

	restarted, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("listen after close: %v", err)
	}
	_ = restarted.Close()
}

func TestListenUnixReplacesStaleSocketWhileHoldingLock(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "agent.sock")
	rawListener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("create stale socket: %v", err)
	}
	rawListener.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := rawListener.Close(); err != nil {
		t.Fatalf("close stale socket: %v", err)
	}

	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("replace stale socket: %v", err)
	}
	_ = listener.Close()
}

func TestListenUnixRefusesWritableOrSymlinkedDirectory(t *testing.T) {
	dir := shortTempDir(t)
	if err := os.Chmod(dir, 0o770); err != nil {
		t.Fatalf("make directory group-writable: %v", err)
	}
	if _, err := ListenUnix(filepath.Join(dir, "agent.sock")); err == nil {
		t.Fatal("expected group-writable directory rejection")
	}

	realDir := shortTempDir(t)
	symlink := filepath.Join(shortTempDir(t), "socket-dir")
	if err := os.Symlink(realDir, symlink); err != nil {
		t.Fatalf("create directory symlink: %v", err)
	}
	if _, err := ListenUnix(filepath.Join(symlink, "agent.sock")); err == nil {
		t.Fatal("expected symlinked directory rejection")
	}
}

func TestListenUnixRefusesWritableAncestor(t *testing.T) {
	ancestor := shortTempDir(t)
	if err := os.Chmod(ancestor, 0o770); err != nil {
		t.Fatalf("make ancestor group-writable: %v", err)
	}
	dir := filepath.Join(ancestor, "safe")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create socket directory: %v", err)
	}

	if _, err := ListenUnix(filepath.Join(dir, "agent.sock")); err == nil {
		t.Fatal("expected writable ancestor rejection")
	}
}

func TestUnixClientRejectsUnhealthyResponse(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "agent.sock")
	listener, err := ListenUnix(socketPath)
	if err != nil {
		t.Fatalf("listen on Unix socket: %v", err)
	}

	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})}
	go httpServer.Serve(listener)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	})

	if err := NewUnixClient(socketPath, time.Second).Health(context.Background()); err == nil {
		t.Fatal("expected unhealthy response error")
	}
}

func TestUnixClientReturnsStructuredActionError(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "agent.sock")
	listener, err := ListenUnix(socketPath)
	if err != nil {
		t.Fatalf("listen on Unix socket: %v", err)
	}

	httpServer := &http.Server{Handler: NewServer(ServerConfig{}).Handler()}
	go httpServer.Serve(listener)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	})

	_, err = NewUnixClient(socketPath, time.Second).Execute(context.Background(), "not.allowlisted", json.RawMessage(`{}`))
	var actionErr *RemoteActionError
	if !errors.As(err, &actionErr) || actionErr.Code != "UNKNOWN_ACTION" || actionErr.StatusCode != http.StatusNotFound {
		t.Fatalf("Execute error = %#v", err)
	}
}

func TestUnixClientClassifiesTimeoutAfterDispatchAsUncertain(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "agent.sock")
	listener, err := ListenUnix(socketPath)
	if err != nil {
		t.Fatalf("listen on Unix socket: %v", err)
	}

	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"action":"agent.health","status":"ok"}`))
	})}
	go httpServer.Serve(listener)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	})

	_, err = NewUnixClient(socketPath, 10*time.Millisecond).ExecuteJob(context.Background(), "agent.health", json.RawMessage(`{}`), "job-key")
	var uncertainError *UncertainActionError
	if !errors.As(err, &uncertainError) {
		t.Fatalf("ExecuteJob error = %#v, want uncertain outcome", err)
	}
}

func shortTempDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "motekar-agent-")
	if err != nil {
		t.Fatalf("create short temp directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
