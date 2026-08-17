package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionLoginCreatesHashedToken(t *testing.T) {
	passwordHash, err := HashPassword("correct horse battery staple", testHashParams())
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	store := &sessionMemoryStore{
		user: SessionUser{
			ID:           "user-id",
			Email:        "owner@example.com",
			PasswordHash: passwordHash,
			IsActive:     true,
		},
	}
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	service := NewSessionService(store).
		WithClock(func() time.Time { return now }).
		WithIDGenerator(func() (string, error) { return "session-id", nil }).
		WithTokenGenerator(func() (string, error) { return "raw-session-token", nil })

	session, err := service.Login(context.Background(), LoginInput{
		Email:     " OWNER@Example.COM ",
		Password:  "correct horse battery staple",
		IPAddress: "127.0.0.1",
		UserAgent: "test-browser",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if session.Token != "raw-session-token" || !session.ExpiresAt.Equal(now.Add(DefaultSessionDuration)) {
		t.Fatalf("unexpected session: %#v", session)
	}
	if store.lookupEmail != "owner@example.com" {
		t.Fatalf("lookup email = %q", store.lookupEmail)
	}
	if store.created.ID != "session-id" || store.created.UserID != "user-id" {
		t.Fatalf("created session = %#v", store.created)
	}
	if store.created.TokenHash == "" || store.created.TokenHash == session.Token {
		t.Fatalf("token was not hashed: %q", store.created.TokenHash)
	}
	if store.created.IPAddress != "127.0.0.1" || store.created.UserAgent != "test-browser" {
		t.Fatalf("request metadata = %#v", store.created)
	}
	store.principal = SessionPrincipal{
		UserID:      store.user.ID,
		Email:       store.user.Email,
		DisplayName: "Owner",
		ExpiresAt:   session.ExpiresAt,
	}
	principal, err := service.Validate(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if principal.UserID != store.user.ID || store.foundTokenHash != store.created.TokenHash || !store.foundAt.Equal(now) {
		t.Fatalf("validated principal = %#v, hash = %q, at = %v", principal, store.foundTokenHash, store.foundAt)
	}
}

func TestSessionLoginReturnsSameErrorForUnknownWrongAndInactiveUsers(t *testing.T) {
	passwordHash, err := HashPassword("correct horse battery staple", testHashParams())
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	cases := []struct {
		name     string
		store    *sessionMemoryStore
		password string
	}{
		{name: "unknown", store: &sessionMemoryStore{findErr: ErrSessionUserNotFound}, password: "anything at all"},
		{
			name: "wrong password",
			store: &sessionMemoryStore{user: SessionUser{
				ID: "user-id", Email: "owner@example.com", PasswordHash: passwordHash, IsActive: true,
			}},
			password: "this password is wrong",
		},
		{
			name: "inactive",
			store: &sessionMemoryStore{user: SessionUser{
				ID: "user-id", Email: "owner@example.com", PasswordHash: passwordHash, IsActive: false,
			}},
			password: "correct horse battery staple",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewSessionService(tc.store).Login(context.Background(), LoginInput{
				Email:    "owner@example.com",
				Password: tc.password,
			})
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("login error = %v, want %v", err, ErrInvalidCredentials)
			}
			if tc.store.created.ID != "" {
				t.Fatalf("unexpected session: %#v", tc.store.created)
			}
		})
	}
}

func TestSessionLogoutDeletesHashedToken(t *testing.T) {
	store := &sessionMemoryStore{}
	service := NewSessionService(store)

	if err := service.Logout(context.Background(), "raw-session-token"); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if store.deletedTokenHash == "" || store.deletedTokenHash == "raw-session-token" {
		t.Fatalf("deleted token hash = %q", store.deletedTokenHash)
	}
	if err := service.Logout(context.Background(), ""); err != nil {
		t.Fatalf("empty logout: %v", err)
	}
}

func TestSessionValidateRejectsMissingSession(t *testing.T) {
	t.Parallel()

	store := &sessionMemoryStore{findSessionErr: ErrInvalidSession}
	service := NewSessionService(store)
	for _, token := range []string{"", "unknown-token"} {
		if _, err := service.Validate(context.Background(), token); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("Validate(%q) error = %v, want %v", token, err, ErrInvalidSession)
		}
	}
}

func TestSessionPasswordChecksHaveBoundedConcurrency(t *testing.T) {
	t.Parallel()

	service := NewSessionService(&sessionMemoryStore{})
	var active atomic.Int32
	var maximum atomic.Int32
	var wait sync.WaitGroup
	for range maxConcurrentPasswordChecks * 3 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := service.runPasswordCheck(context.Background(), func() (bool, error) {
				current := active.Add(1)
				for {
					previous := maximum.Load()
					if current <= previous || maximum.CompareAndSwap(previous, current) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				active.Add(-1)
				return true, nil
			})
			if err != nil {
				t.Errorf("password check: %v", err)
			}
		}()
	}
	wait.Wait()
	if maximum.Load() > maxConcurrentPasswordChecks {
		t.Fatalf("maximum concurrent password checks = %d", maximum.Load())
	}
}

type sessionMemoryStore struct {
	user             SessionUser
	findErr          error
	lookupEmail      string
	created          SessionRecord
	principal        SessionPrincipal
	findSessionErr   error
	foundTokenHash   string
	foundAt          time.Time
	deletedTokenHash string
}

func (s *sessionMemoryStore) FindUserByEmail(_ context.Context, email string) (SessionUser, error) {
	s.lookupEmail = email
	if s.findErr != nil {
		return SessionUser{}, s.findErr
	}
	return s.user, nil
}

func (s *sessionMemoryStore) CreateSession(_ context.Context, session SessionRecord) error {
	s.created = session
	return nil
}

func (s *sessionMemoryStore) FindActiveSession(_ context.Context, tokenHash string, now time.Time) (SessionPrincipal, error) {
	s.foundTokenHash = tokenHash
	s.foundAt = now
	return s.principal, s.findSessionErr
}

func (s *sessionMemoryStore) DeleteSessionByTokenHash(_ context.Context, tokenHash string) error {
	s.deletedTokenHash = tokenHash
	return nil
}
