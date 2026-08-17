package agent

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
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

	httpServer := &http.Server{Handler: NewServer(ServerConfig{}).Handler()}
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
	if len(capabilities.Actions) != 2 || capabilities.Actions[0] != "agent.capabilities" || capabilities.Actions[1] != "agent.health" {
		t.Fatalf("capabilities = %#v", capabilities.Actions)
	}
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

func shortTempDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "motekar-agent-")
	if err != nil {
		t.Fatalf("create short temp directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
