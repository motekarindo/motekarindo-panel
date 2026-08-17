package auth

import (
	"context"
	"errors"
	"testing"
)

func TestCreateFirstAdminCreatesHashedAdmin(t *testing.T) {
	store := &bootstrapMemoryStore{}
	service := NewBootstrapService(store).
		WithIDGenerator(func() (string, error) { return "admin-id", nil }).
		WithPasswordHashParams(testHashParams())

	admin, err := service.CreateFirstAdmin(context.Background(), BootstrapInput{
		Email:       " OWNER@Example.COM ",
		DisplayName: " Owner ",
		Password:    "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateFirstAdmin returned error: %v", err)
	}

	if admin.ID != "admin-id" || admin.Email != "owner@example.com" || admin.DisplayName != "Owner" {
		t.Fatalf("unexpected admin: %#v", admin)
	}
	if admin.PasswordHash == "" || admin.PasswordHash == "correct horse battery staple" {
		t.Fatalf("password was not hashed: %q", admin.PasswordHash)
	}

	ok, err := VerifyPassword("correct horse battery staple", admin.PasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected stored hash to verify")
	}
}

func TestCreateFirstAdminRejectsDuplicateBootstrap(t *testing.T) {
	store := &bootstrapMemoryStore{admins: []BootstrapAdmin{{ID: "existing"}}}
	service := NewBootstrapService(store).WithPasswordHashParams(testHashParams())

	_, err := service.CreateFirstAdmin(context.Background(), BootstrapInput{
		Email:       "owner@example.com",
		DisplayName: "Owner",
		Password:    "correct horse battery staple",
	})
	if !errors.Is(err, ErrAdminAlreadyExists) {
		t.Fatalf("expected ErrAdminAlreadyExists, got %v", err)
	}
}

func TestCreateFirstAdminValidatesInput(t *testing.T) {
	cases := []struct {
		name  string
		input BootstrapInput
		want  error
	}{
		{
			name:  "invalid email",
			input: BootstrapInput{Email: "not-email", DisplayName: "Owner", Password: "correct horse battery staple"},
			want:  ErrInvalidEmail,
		},
		{
			name:  "missing display name",
			input: BootstrapInput{Email: "owner@example.com", Password: "correct horse battery staple"},
			want:  ErrInvalidDisplayName,
		},
		{
			name:  "weak password",
			input: BootstrapInput{Email: "owner@example.com", DisplayName: "Owner", Password: "short"},
			want:  ErrWeakPassword,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := NewBootstrapService(&bootstrapMemoryStore{}).WithPasswordHashParams(testHashParams())
			_, err := service.CreateFirstAdmin(context.Background(), tc.input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func testHashParams() PasswordHashParams {
	return PasswordHashParams{
		MemoryKB:    1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  8,
		KeyLength:   16,
	}
}

type bootstrapMemoryStore struct {
	admins []BootstrapAdmin
}

func (s *bootstrapMemoryStore) CreateAdmin(_ context.Context, admin BootstrapAdmin) error {
	if len(s.admins) > 0 {
		return ErrAdminAlreadyExists
	}
	s.admins = append(s.admins, admin)
	return nil
}
