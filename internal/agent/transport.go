package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxAgentResponseBytes = 1 << 20

var ErrSocketInUse = errors.New("agent socket is already in use")

func ListenUnix(socketPath string) (net.Listener, error) {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		return nil, errors.New("agent socket path is required")
	}

	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create agent socket directory: %w", err)
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect agent socket directory: %w", err)
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("agent socket directory %q must be a real directory", dir)
	}
	stat, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("agent socket directory %q must be owned by the agent user", dir)
	}
	if dirInfo.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("agent socket directory %q must not be writable by group or others", dir)
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve agent socket directory: %w", err)
	}
	resolvedDir, err = filepath.Abs(resolvedDir)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute agent socket directory: %w", err)
	}
	if err := validateSocketAncestors(resolvedDir); err != nil {
		return nil, err
	}
	socketPath = filepath.Join(resolvedDir, filepath.Base(socketPath))

	lock, err := acquireSocketLock(socketPath + ".lock")
	if err != nil {
		return nil, err
	}
	releaseLock := true
	defer func() {
		if releaseLock {
			releaseSocketLock(lock)
		}
	}()

	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("agent socket path %q exists and is not a socket", socketPath)
		}
		conn, dialErr := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, ErrSocketInUse
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("remove stale agent socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect agent socket path: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on agent socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o660); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("set agent socket permissions: %w", err)
	}
	releaseLock = false
	return &lockedListener{Listener: listener, lock: lock}, nil
}

func validateSocketAncestors(dir string) error {
	euid := uint32(os.Geteuid())
	for current := dir; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect agent socket ancestor %q: %w", current, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("agent socket ancestor %q must be a real directory", current)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (stat.Uid != 0 && stat.Uid != euid) {
			return fmt.Errorf("agent socket ancestor %q must be owned by root or the agent user", current)
		}
		if info.Mode().Perm()&0o022 != 0 && !(stat.Uid == 0 && info.Mode()&os.ModeSticky != 0) {
			return fmt.Errorf("agent socket ancestor %q has unsafe write permissions", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}

func acquireSocketLock(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open agent socket lock: %w", err)
	}
	lock := os.NewFile(uintptr(fd), path)
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrSocketInUse
		}
		return nil, fmt.Errorf("lock agent socket: %w", err)
	}
	return lock, nil
}

func releaseSocketLock(lock *os.File) {
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	_ = lock.Close()
}

type lockedListener struct {
	net.Listener
	lock     *os.File
	once     sync.Once
	closeErr error
}

func (l *lockedListener) Close() error {
	l.once.Do(func() {
		l.closeErr = l.Listener.Close()
		releaseSocketLock(l.lock)
	})
	return l.closeErr
}

type UnixClient struct {
	httpClient *http.Client
}

func NewUnixClient(socketPath string, timeout time.Duration) *UnixClient {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &UnixClient{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}
}

func (c *UnixClient) Health(ctx context.Context) error {
	var response struct {
		Status string `json:"status"`
	}
	if err := c.getJSON(ctx, "/healthz", &response); err != nil {
		return err
	}
	if response.Status != "ok" {
		return fmt.Errorf("agent health status is %q", response.Status)
	}
	return nil
}

func (c *UnixClient) Capabilities(ctx context.Context) (Capabilities, error) {
	var capabilities Capabilities
	if err := c.getJSON(ctx, "/capabilities", &capabilities); err != nil {
		return Capabilities{}, err
	}
	return capabilities, nil
}

func (c *UnixClient) getJSON(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+path, nil)
	if err != nil {
		return err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request agent %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxAgentResponseBytes))
		return fmt.Errorf("agent %s returned HTTP %d", path, response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxAgentResponseBytes)).Decode(target); err != nil {
		return fmt.Errorf("decode agent %s response: %w", path, err)
	}
	return nil
}
