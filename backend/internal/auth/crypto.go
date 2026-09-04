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
var errCipherNotConfigured = errors.New("field cipher is not configured")

// FieldCipher is an AES-256-GCM keyring for column-level PII. Ciphertext
// written by Seal is prefixed with a one-byte key version, so keys can be
// rotated: add the new key as the primary, keep the old ones for reads, and
// run cmd/reencrypt to migrate existing rows. Blobs written before versioning
// carry no prefix and are decrypted with the legacy key as a fallback.
type FieldCipher struct {
	keys    map[byte][]byte // version (1-255) -> 32-byte key
	primary byte            // version Seal writes with
	legacy  []byte          // key for unversioned ciphertext, nil to disable
}

// NewFieldCipher builds a single-key cipher. It writes version 1 and also
// accepts unversioned legacy ciphertext under the same key.
func NewFieldCipher(key []byte) (*FieldCipher, error) {
	if len(key) != 32 {
		return nil, errors.New("field cipher key must be 32 bytes")
	}
	k := append([]byte(nil), key...)
	return &FieldCipher{keys: map[byte][]byte{1: k}, primary: 1, legacy: k}, nil
}

// NewFieldCipherRing builds a rotating keyring. keys maps each version byte
// (1-255) to a 32-byte key; primary is the version Seal writes; legacy (32
// bytes, or nil) decrypts pre-versioning ciphertext. primary must be in keys.
func NewFieldCipherRing(primary byte, keys map[byte][]byte, legacy []byte) (*FieldCipher, error) {
	if primary == 0 {
		return nil, errors.New("primary key version must be 1-255")
	}
	if len(keys[primary]) != 32 {
		return nil, errors.New("primary key is missing or not 32 bytes")
	}
	ring := make(map[byte][]byte, len(keys))
	for v, k := range keys {
		if v == 0 || len(k) != 32 {
			return nil, fmt.Errorf("field cipher key version %d must be 1-255 with a 32-byte key", v)
		}
		ring[v] = append([]byte(nil), k...)
	}
	var lg []byte
	if len(legacy) == 32 {
		lg = append([]byte(nil), legacy...)
	}
	return &FieldCipher{keys: ring, primary: primary, legacy: lg}, nil
}

// PrimaryVersion is the key version Seal currently writes with.
func (c *FieldCipher) PrimaryVersion() byte {
	if c == nil {
		return 0
	}
	return c.primary
}

// NeedsRotation reports whether ciphertext was written with a key other than
// the current primary (including unversioned legacy blobs).
func (c *FieldCipher) NeedsRotation(ciphertext []byte) bool {
	if c == nil {
		return false
	}
	return len(ciphertext) == 0 || ciphertext[0] != c.primary
}

func gcmFor(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create field cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func (c *FieldCipher) Seal(plaintext string) ([]byte, error) {
	if c == nil || len(c.keys[c.primary]) != 32 {
		return nil, errCipherNotConfigured
	}
	gcm, err := gcmFor(c.keys[c.primary])
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate field cipher nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return append([]byte{c.primary}, sealed...), nil
}

func (c *FieldCipher) Open(ciphertext []byte) (string, error) {
	if c == nil || len(c.keys) == 0 {
		return "", errCipherNotConfigured
	}
	// Versioned layout: [version][nonce][gcm ciphertext+tag].
	if len(ciphertext) >= 1 {
		if key, ok := c.keys[ciphertext[0]]; ok {
			if pt, err := gcmOpen(key, ciphertext[1:]); err == nil {
				return string(pt), nil
			}
		}
	}
	// Fallback: unversioned legacy layout.
	if c.legacy != nil {
		if pt, err := gcmOpen(c.legacy, ciphertext); err == nil {
			return string(pt), nil
		}
	}
	return "", errInvalidCiphertext
}

func gcmOpen(key, blob []byte) ([]byte, error) {
	gcm, err := gcmFor(key)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errInvalidCiphertext
	}
	nonce, payload := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	return gcm.Open(nil, nonce, payload, nil)
}

func LookupHash(key []byte, normalizedValue string) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("lookup HMAC key must be 32 bytes")
	}
	return hmacSum(key, normalizedValue), nil
}

func hmacSum(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

// LookupHasher is an HMAC-SHA-256 keyring for the deterministic lookup hashes
// stored on UNIQUE columns (account email, OAuth subject). Writes use the
// primary key; reads try the primary plus any retired keys, so a key can be
// rotated without a flag day: promote the new key to primary, keep the old one
// as a secondary read key, run cmd/reencrypt to rewrite the stored hashes it
// can (those whose plaintext is recoverable), then drop the secondary.
type LookupHasher struct {
	primary   []byte
	secondary [][]byte
}

// NewLookupHasher builds a hasher. primary must be 32 bytes; each secondary key
// (retired, read-only) must also be 32 bytes. A nil primary yields a nil
// hasher, matching the "not configured" state elsewhere in the package.
func NewLookupHasher(primary []byte, secondary ...[]byte) (*LookupHasher, error) {
	if primary == nil {
		return nil, nil
	}
	if len(primary) != 32 {
		return nil, errors.New("lookup HMAC primary key must be 32 bytes")
	}
	h := &LookupHasher{primary: append([]byte(nil), primary...)}
	for _, k := range secondary {
		if len(k) != 32 {
			return nil, errors.New("lookup HMAC secondary key must be 32 bytes")
		}
		h.secondary = append(h.secondary, append([]byte(nil), k...))
	}
	return h, nil
}

// Hash returns the primary hash, used for writes.
func (h *LookupHasher) Hash(normalizedValue string) []byte {
	return hmacSum(h.primary, normalizedValue)
}

// Candidates returns the primary hash followed by one per retired key. Match a
// stored column against the whole slice (`col = ANY($1)`) so rows written under
// an older key still resolve.
func (h *LookupHasher) Candidates(normalizedValue string) [][]byte {
	out := make([][]byte, 0, 1+len(h.secondary))
	out = append(out, hmacSum(h.primary, normalizedValue))
	for _, k := range h.secondary {
		out = append(out, hmacSum(k, normalizedValue))
	}
	return out
}

// NeedsRotation reports whether a stored hash was produced by a retired key and
// should be rewritten with the primary.
func (h *LookupHasher) NeedsRotation(stored []byte, normalizedValue string) bool {
	return !hmac.Equal(stored, h.Hash(normalizedValue))
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
