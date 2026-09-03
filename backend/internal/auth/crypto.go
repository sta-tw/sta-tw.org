package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

var errInvalidCiphertext = errors.New("invalid encrypted value")

type FieldCipher struct {
	key []byte
}

func NewFieldCipher(key []byte) (*FieldCipher, error) {
	if len(key) != 32 {
		return nil, errors.New("field cipher key must be 32 bytes")
	}
	copyKey := append([]byte(nil), key...)
	return &FieldCipher{key: copyKey}, nil
}

func (c *FieldCipher) Seal(plaintext string) ([]byte, error) {
	if c == nil || len(c.key) != 32 {
		return nil, errors.New("field cipher is not configured")
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, fmt.Errorf("create field cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create field cipher mode: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate field cipher nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func (c *FieldCipher) Open(ciphertext []byte) (string, error) {
	if c == nil || len(c.key) != 32 {
		return "", errors.New("field cipher is not configured")
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", fmt.Errorf("create field cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create field cipher mode: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", errInvalidCiphertext
	}
	nonce, payload := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", errInvalidCiphertext
	}
	return string(plaintext), nil
}

func LookupHash(key []byte, normalizedValue string) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("lookup HMAC key must be 32 bytes")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(normalizedValue))
	return mac.Sum(nil), nil
}

func HashOpaqueToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func NewOpaqueToken(bytes int) (string, error) {
	if bytes < 32 {
		bytes = 32
	}
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate opaque token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func ConstantTimeStringEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
