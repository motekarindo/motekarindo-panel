package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const DefaultSessionDuration = 24 * time.Hour

const maxConcurrentPasswordChecks = 4

var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrInvalidSession      = errors.New("invalid session")
	ErrSessionUserNotFound = errors.New("session user not found")
)

type LoginInput struct {
	Email     string
	Password  string
	IPAddress string
	UserAgent string
}

type SessionUser struct {
	ID           string
	Email        string
	PasswordHash string
	IsActive     bool
}

type SessionRecord struct {
	ID        string
	UserID    string
	TokenHash string
	IPAddress string
	UserAgent string
	ExpiresAt time.Time
}

type LoginSession struct {
	Token     string
	ExpiresAt time.Time
}

type SessionPrincipal struct {
	UserID      string
	Email       string
	DisplayName string
	ExpiresAt   time.Time
}

type SessionStore interface {
	FindUserByEmail(ctx context.Context, email string) (SessionUser, error)
	CreateSession(ctx context.Context, session SessionRecord) error
	FindActiveSession(ctx context.Context, tokenHash string, now time.Time) (SessionPrincipal, error)
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error
}

type SessionService struct {
	store         SessionStore
	now           func() time.Time
	newID         func() (string, error)
	newToken      func() (string, error)
	passwordSlots chan struct{}
}

func NewSessionService(store SessionStore) SessionService {
	return SessionService{
		store:         store,
		now:           func() time.Time { return time.Now().UTC() },
		newID:         newUUID,
		newToken:      newSessionToken,
		passwordSlots: make(chan struct{}, maxConcurrentPasswordChecks),
	}
}

func (s SessionService) WithClock(now func() time.Time) SessionService {
	s.now = now
	return s
}

func (s SessionService) WithIDGenerator(newID func() (string, error)) SessionService {
	s.newID = newID
	return s
}

func (s SessionService) WithTokenGenerator(newToken func() (string, error)) SessionService {
	s.newToken = newToken
	return s
}

func (s SessionService) Login(ctx context.Context, input LoginInput) (LoginSession, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	user, err := s.store.FindUserByEmail(ctx, email)
	if errors.Is(err, ErrSessionUserNotFound) {
		_, checkErr := s.runPasswordCheck(ctx, func() (bool, error) {
			runDummyPasswordCheck(input.Password)
			return false, nil
		})
		if checkErr != nil {
			return LoginSession{}, checkErr
		}
		return LoginSession{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginSession{}, err
	}

	validPassword, err := s.runPasswordCheck(ctx, func() (bool, error) {
		return VerifyPassword(input.Password, user.PasswordHash)
	})
	if err != nil {
		return LoginSession{}, fmt.Errorf("verify password: %w", err)
	}
	if !validPassword || !user.IsActive {
		return LoginSession{}, ErrInvalidCredentials
	}

	id, err := s.newID()
	if err != nil {
		return LoginSession{}, err
	}
	token, err := s.newToken()
	if err != nil {
		return LoginSession{}, err
	}
	expiresAt := s.now().Add(DefaultSessionDuration)
	if err := s.store.CreateSession(ctx, SessionRecord{
		ID:        id,
		UserID:    user.ID,
		TokenHash: hashSessionToken(token),
		IPAddress: strings.TrimSpace(input.IPAddress),
		UserAgent: strings.TrimSpace(input.UserAgent),
		ExpiresAt: expiresAt,
	}); err != nil {
		return LoginSession{}, err
	}

	return LoginSession{Token: token, ExpiresAt: expiresAt}, nil
}

func (s SessionService) runPasswordCheck(ctx context.Context, check func() (bool, error)) (bool, error) {
	select {
	case s.passwordSlots <- struct{}{}:
		defer func() { <-s.passwordSlots }()
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return check()
}

func (s SessionService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSessionByTokenHash(ctx, hashSessionToken(token))
}

func (s SessionService) Validate(ctx context.Context, token string) (SessionPrincipal, error) {
	if strings.TrimSpace(token) == "" {
		return SessionPrincipal{}, ErrInvalidSession
	}
	principal, err := s.store.FindActiveSession(ctx, hashSessionToken(token), s.now().UTC())
	if errors.Is(err, ErrInvalidSession) {
		return SessionPrincipal{}, ErrInvalidSession
	}
	if err != nil {
		return SessionPrincipal{}, fmt.Errorf("find active session: %w", err)
	}
	return principal, nil
}

func newSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashSessionToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func runDummyPasswordCheck(password string) {
	params := DefaultPasswordHashParams()
	salt := []byte("motekar-dummy-v1")
	_ = argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKB, params.Parallelism, params.KeyLength)
}
