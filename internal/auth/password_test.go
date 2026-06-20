package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("secret-password", testHashParams())
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if !strings.HasPrefix(hash, "argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %q", hash)
	}

	ok, err := VerifyPassword("secret-password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	ok, err = VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword returned error for wrong password: %v", err)
	}
	if ok {
		t.Fatal("wrong password should not verify")
	}
}

func TestVerifyPasswordRejectsInvalidHash(t *testing.T) {
	if _, err := VerifyPassword("secret-password", "not-a-valid-hash"); !errors.Is(err, ErrInvalidPasswordHash) {
		t.Fatalf("expected ErrInvalidPasswordHash, got %v", err)
	}
}
