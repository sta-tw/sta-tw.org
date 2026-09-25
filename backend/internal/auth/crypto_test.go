package auth

import (
	"bytes"
	"testing"
)

func TestFieldCipherRoundTrip(t *testing.T) {
	cipher, err := NewFieldCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	ciphertext, err := cipher.Seal("student@example.test")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	plaintext, err := cipher.Open(ciphertext)
	if err != nil || plaintext != "student@example.test" {
		t.Fatalf("Open() = %q, %v", plaintext, err)
	}
	if bytes.Equal(ciphertext, []byte("student@example.test")) {
		t.Fatal("ciphertext contains plaintext")
	}
}

func TestFieldCipherRejectsTampering(t *testing.T) {
	cipher, err := NewFieldCipher(bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	ciphertext, err := cipher.Seal("sensitive")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := cipher.Open(ciphertext); err == nil {
		t.Fatal("Open() error = nil, want tampering error")
	}
}

func TestFieldCipherRingRotation(t *testing.T) {
	k1 := bytes.Repeat([]byte{1}, 32)
	k2 := bytes.Repeat([]byte{2}, 32)

	v1, err := NewFieldCipherRing(1, map[byte][]byte{1: k1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	old, err := v1.Seal("secret value")
	if err != nil {
		t.Fatal(err)
	}
	if old[0] != 1 {
		t.Fatalf("expected version-1 prefix, got %d", old[0])
	}

	// After rotation the ring writes v2 but still reads v1.
	v2, err := NewFieldCipherRing(2, map[byte][]byte{1: k1, 2: k2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v2.Open(old)
	if err != nil || got != "secret value" {
		t.Fatalf("v2 reading v1 blob: %q %v", got, err)
	}
	if !v2.NeedsRotation(old) {
		t.Fatal("v1 blob should need rotation under a v2 primary")
	}
	fresh, err := v2.Seal("secret value")
	if err != nil {
		t.Fatal(err)
	}
	if fresh[0] != 2 || v2.NeedsRotation(fresh) {
		t.Fatalf("v2 write not marked current: prefix %d", fresh[0])
	}
	// A ring missing the old key cannot read the old blob.
	only2, _ := NewFieldCipherRing(2, map[byte][]byte{2: k2}, nil)
	if _, err := only2.Open(old); err == nil {
		t.Fatal("expected failure decrypting v1 blob without v1 key")
	}
}

func TestFieldCipherLegacyFallback(t *testing.T) {
	key := bytes.Repeat([]byte{5}, 32)
	// Simulate a pre-versioning blob: raw gcm output with no version prefix.
	raw, err := gcmSealForTest(key, "legacy secret")
	if err != nil {
		t.Fatal(err)
	}
	ring, err := NewFieldCipherRing(9, map[byte][]byte{9: bytes.Repeat([]byte{6}, 32)}, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ring.Open(raw)
	if err != nil || got != "legacy secret" {
		t.Fatalf("legacy fallback: %q %v", got, err)
	}
	if !ring.NeedsRotation(raw) {
		t.Fatal("legacy blob should need rotation")
	}
}

func gcmSealForTest(key []byte, plaintext string) ([]byte, error) {
	gcm, err := gcmFor(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func TestLookupHashIsDeterministic(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	first, err := LookupHash(key, NormalizeEmail(" Student@Example.Test "))
	if err != nil {
		t.Fatalf("LookupHash() error = %v", err)
	}
	second, err := LookupHash(key, NormalizeEmail("student@example.test"))
	if err != nil {
		t.Fatalf("LookupHash() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("normalized equivalent emails should have the same lookup hash")
	}
}

func TestLookupHasherRotation(t *testing.T) {
	oldKey := bytes.Repeat([]byte{1}, 32)
	newKey := bytes.Repeat([]byte{2}, 32)

	oldHasher, err := NewLookupHasher(oldKey)
	if err != nil {
		t.Fatalf("NewLookupHasher(old) = %v", err)
	}
	// Primary rotated to newKey, oldKey kept as a read-only secondary.
	rotated, err := NewLookupHasher(newKey, oldKey)
	if err != nil {
		t.Fatalf("NewLookupHasher(rotated) = %v", err)
	}

	const value = "student@example.test"
	legacy := oldHasher.Hash(value)

	// The rotated hasher writes with the new key...
	if bytes.Equal(rotated.Hash(value), legacy) {
		t.Fatal("rotated hasher still writes the legacy hash")
	}
	// ...but a legacy hash is still one of its read candidates...
	found := false
	for _, c := range rotated.Candidates(value) {
		if bytes.Equal(c, legacy) {
			found = true
		}
	}
	if !found {
		t.Fatal("legacy hash is not among the rotated hasher's candidates")
	}
	// ...and it is flagged for rewriting.
	if !rotated.NeedsRotation(legacy, value) {
		t.Fatal("legacy hash should need rotation")
	}
	if rotated.NeedsRotation(rotated.Hash(value), value) {
		t.Fatal("a primary hash should not need rotation")
	}
}

func TestNewLookupHasherRejectsShortKeys(t *testing.T) {
	if _, err := NewLookupHasher(bytes.Repeat([]byte{1}, 16)); err == nil {
		t.Fatal("expected error for short primary key")
	}
	if _, err := NewLookupHasher(bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 8)); err == nil {
		t.Fatal("expected error for short secondary key")
	}
	if h, err := NewLookupHasher(nil); err != nil || h != nil {
		t.Fatalf("NewLookupHasher(nil) = %v, %v; want nil, nil", h, err)
	}
}
