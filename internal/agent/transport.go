package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

type RemoteActionError struct {
	StatusCode int
	Code       string
}

type UncertainActionError struct {
	err error
}

func (e *UncertainActionError) Error() string { return "agent action outcome is uncertain" }
func (e *UncertainActionError) Unwrap() error { return e.err }

type ProtocolError struct {
	err error
}

func (e *ProtocolError) Error() string { return e.err.Error() }
func (e *ProtocolError) Unwrap() error { return e.err }

func (e *RemoteActionError) Error() string {
	return fmt.Sprintf("agent action failed with %s (HTTP %d)", e.Code, e.StatusCode)
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

func (c *UnixClient) Execute(ctx context.Context, name string, payload json.RawMessage) (ActionResult, error) {
	return c.ExecuteJob(ctx, name, payload, "")
}

func (c *UnixClient) ExecuteJob(ctx context.Context, name string, payload json.RawMessage, idempotencyKey string) (ActionResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ActionResult{}, &ProtocolError{err: errors.New("agent action name is required")}
	}
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || len(payload) > maxActionPayloadBytes || payload[0] != '{' || !json.Valid(payload) {
		return ActionResult{}, &ProtocolError{err: errors.New("invalid agent action payload")}
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > maxIdempotencyKeyBytes {
		return ActionResult{}, &ProtocolError{err: errors.New("invalid agent action idempotency key")}
	}
	body := make([]byte, 0, len(payload)+12)
	body = append(body, `{"payload":`...)
	body = append(body, payload...)
	body = append(body, '}')
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://agent/actions/"+url.PathEscape(name),
		bytes.NewReader(body),
	)
	if err != nil {
		return ActionResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	var wroteRequest atomic.Bool
	trace := &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { wroteRequest.Store(true) }}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	response, err := c.httpClient.Do(request)
	if err != nil {
		if wroteRequest.Load() {
			return ActionResult{}, &UncertainActionError{err: err}
		}
		return ActionResult{}, fmt.Errorf("request agent action: %w", err)
	}
	defer response.Body.Close()

	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxAgentResponseBytes+1))
	if err != nil {
		return ActionResult{}, &UncertainActionError{err: err}
	}
	if len(encoded) > maxAgentResponseBytes {
		return ActionResult{}, &ProtocolError{err: errors.New("agent action response exceeds size limit")}
	}
	if response.StatusCode != http.StatusOK {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		code := "HTTP_ERROR"
		if json.Unmarshal(encoded, &envelope) == nil && envelope.Error.Code != "" {
			code = envelope.Error.Code
		}
		return ActionResult{}, &RemoteActionError{StatusCode: response.StatusCode, Code: code}
	}

	var result ActionResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		return ActionResult{}, &ProtocolError{err: errors.New("agent returned malformed action result")}
	}
	if result.Action != name || result.Status != "ok" {
		return ActionResult{}, &ProtocolError{err: errors.New("agent returned an invalid action result")}
	}
	return result, nil
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
