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

const (
	passwordMemory      = 64 * 1024
	passwordIterations  = 3
	passwordParallelism = 2
	passwordSaltBytes   = 16
	passwordKeyBytes    = 32
)

var errInvalidPasswordHash = errors.New("invalid password hash")

func HashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemory, passwordParallelism, passwordKeyBytes)
	encode := base64.RawStdEncoding
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		passwordMemory,
		passwordIterations,
		passwordParallelism,
		encode.EncodeToString(salt),
		encode.EncodeToString(hash),
	), nil
}

func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return false, errInvalidPasswordHash
	}
	params, err := parsePasswordParams(parts[2])
	if err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, errInvalidPasswordHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false, errInvalidPasswordHash
	}
	got := argon2.IDKey([]byte(password), salt, params.iterations, params.memory, params.parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

type passwordParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parsePasswordParams(raw string) (passwordParams, error) {
	result := passwordParams{}
	for _, part := range strings.Split(raw, ",") {
		keyValue := strings.SplitN(part, "=", 2)
		if len(keyValue) != 2 {
			return passwordParams{}, errInvalidPasswordHash
		}
		value, err := strconv.ParseUint(keyValue[1], 10, 32)
		if err != nil || value == 0 {
			return passwordParams{}, errInvalidPasswordHash
		}
		switch keyValue[0] {
		case "m":
			result.memory = uint32(value)
		case "t":
			result.iterations = uint32(value)
		case "p":
			if value > 255 {
				return passwordParams{}, errInvalidPasswordHash
			}
			result.parallelism = uint8(value)
		default:
			return passwordParams{}, errInvalidPasswordHash
		}
	}
	if result.memory < 16*1024 || result.memory > 1024*1024 || result.iterations < 1 || result.iterations > 10 || result.parallelism < 1 || result.parallelism > 8 {
		return passwordParams{}, errInvalidPasswordHash
	}
	return result, nil
}
