package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

const MinBootstrapPasswordLength = 12

var (
	ErrAdminAlreadyExists = errors.New("admin user already exists")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidDisplayName = errors.New("invalid display name")
	ErrWeakPassword       = errors.New("password is too weak")
)

type BootstrapInput struct {
	Email       string
	DisplayName string
	Password    string
}

type BootstrapAdmin struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
}

type BootstrapStore interface {
	AdminCount(ctx context.Context) (int, error)
	CreateAdmin(ctx context.Context, admin BootstrapAdmin) error
}

type BootstrapService struct {
	store      BootstrapStore
	hashParams PasswordHashParams
	newID      func() (string, error)
}

func NewBootstrapService(store BootstrapStore) BootstrapService {
	return BootstrapService{
		store:      store,
		hashParams: DefaultPasswordHashParams(),
		newID:      newUUID,
	}
}

func (s BootstrapService) WithPasswordHashParams(params PasswordHashParams) BootstrapService {
	s.hashParams = params
	return s
}

func (s BootstrapService) WithIDGenerator(newID func() (string, error)) BootstrapService {
	s.newID = newID
	return s
}

func (s BootstrapService) CreateFirstAdmin(ctx context.Context, input BootstrapInput) (BootstrapAdmin, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return BootstrapAdmin{}, err
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		return BootstrapAdmin{}, ErrInvalidDisplayName
	}
	if len(input.Password) < MinBootstrapPasswordLength {
		return BootstrapAdmin{}, ErrWeakPassword
	}

	count, err := s.store.AdminCount(ctx)
	if err != nil {
		return BootstrapAdmin{}, err
	}
	if count > 0 {
		return BootstrapAdmin{}, ErrAdminAlreadyExists
	}

	id, err := s.newID()
	if err != nil {
		return BootstrapAdmin{}, err
	}
	passwordHash, err := HashPassword(input.Password, s.hashParams)
	if err != nil {
		return BootstrapAdmin{}, err
	}

	admin := BootstrapAdmin{
		ID:           id,
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
	}
	if err := s.store.CreateAdmin(ctx, admin); err != nil {
		return BootstrapAdmin{}, err
	}

	return admin, nil
}

func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", ErrInvalidEmail
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return "", ErrInvalidEmail
	}
	return value, nil
}

func newUUID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}
