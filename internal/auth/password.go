package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const passwordHashAlgorithm = "argon2id"

var ErrInvalidPasswordHash = errors.New("invalid password hash")

type PasswordHashParams struct {
	MemoryKB    uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

func DefaultPasswordHashParams() PasswordHashParams {
	return PasswordHashParams{
		MemoryKB:    64 * 1024,
		Iterations:  3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
}

func HashPassword(password string, params PasswordHashParams) (string, error) {
	if err := validateHashParams(params); err != nil {
		return "", err
	}

	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	key := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKB, params.Parallelism, params.KeyLength)
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedKey := base64.RawStdEncoding.EncodeToString(key)

	return fmt.Sprintf(
		"%s$v=19$m=%d,t=%d,p=%d$%s$%s",
		passwordHashAlgorithm,
		params.MemoryKB,
		params.Iterations,
		params.Parallelism,
		encodedSalt,
		encodedKey,
	), nil
}

func VerifyPassword(password, encodedHash string) (bool, error) {
	params, salt, expectedKey, err := decodePasswordHash(encodedHash)
	if err != nil {
		return false, err
	}

	actualKey := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKB, params.Parallelism, params.KeyLength)
	return subtle.ConstantTimeCompare(actualKey, expectedKey) == 1, nil
}

func validateHashParams(params PasswordHashParams) error {
	if params.MemoryKB == 0 || params.Iterations == 0 || params.Parallelism == 0 || params.SaltLength == 0 || params.KeyLength == 0 {
		return errors.New("password hash params must be non-zero")
	}
	return nil
}

func decodePasswordHash(encodedHash string) (PasswordHashParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 5 || parts[0] != passwordHashAlgorithm || parts[1] != "v=19" {
		return PasswordHashParams{}, nil, nil, ErrInvalidPasswordHash
	}

	params, err := parseParamList(parts[2])
	if err != nil {
		return PasswordHashParams{}, nil, nil, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return PasswordHashParams{}, nil, nil, ErrInvalidPasswordHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return PasswordHashParams{}, nil, nil, ErrInvalidPasswordHash
	}

	hashParams := PasswordHashParams{
		MemoryKB:    uint32(params["m"]),
		Iterations:  uint32(params["t"]),
		Parallelism: uint8(params["p"]),
		SaltLength:  uint32(len(salt)),
		KeyLength:   uint32(len(key)),
	}
	if err := validateHashParams(hashParams); err != nil {
		return PasswordHashParams{}, nil, nil, ErrInvalidPasswordHash
	}

	return hashParams, salt, key, nil
}

func parseParamList(raw string) (map[string]int, error) {
	out := make(map[string]int)
	for _, part := range strings.Split(raw, ",") {
		keyValue := strings.SplitN(part, "=", 2)
		if len(keyValue) != 2 {
			return nil, ErrInvalidPasswordHash
		}
		value, err := strconv.Atoi(keyValue[1])
		if err != nil || value <= 0 {
			return nil, ErrInvalidPasswordHash
		}
		out[keyValue[0]] = value
	}
	for _, key := range []string{"m", "t", "p"} {
		if out[key] == 0 {
			return nil, ErrInvalidPasswordHash
		}
	}
	return out, nil
}
