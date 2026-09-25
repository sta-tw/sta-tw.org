//go:build integration

package main

import (
	"bytes"
	"context"
	"testing"

	"sta-backend/internal/auth"
	"sta-backend/internal/dbtest"
)

func TestRotateColumnMigratesToPrimary(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	k1 := bytes.Repeat([]byte{1}, 32)
	k2 := bytes.Repeat([]byte{2}, 32)

	v1, err := auth.NewFieldCipherRing(1, map[byte][]byte{1: k1}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Seed three accounts whose email_ciphertext is written with key v1.
	ids := make([]string, 3)
	for i := range ids {
		id := dbtest.InsertAccount(t, ctx, pool, "")
		ids[i] = id.String()
		sealed, err := v1.Seal("user" + id.String() + "@example.test")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE accounts SET email_ciphertext = $2 WHERE id = $1`, id, sealed); err != nil {
			t.Fatal(err)
		}
	}

	v2, err := auth.NewFieldCipherRing(2, map[byte][]byte{1: k1, 2: k2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tgt := target{table: "accounts", column: "email_ciphertext"}

	// Dry run: reports stale rows, changes nothing.
	scanned, stale, err := rotateColumn(ctx, pool, v2, tgt, false, 100)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if stale < 3 {
		t.Fatalf("dry run stale = %d, want >= 3", stale)
	}
	if scanned < 3 {
		t.Fatalf("dry run scanned = %d", scanned)
	}
	for _, id := range ids {
		var ct []byte
		if err := pool.QueryRow(ctx, `SELECT email_ciphertext FROM accounts WHERE id = $1`, id).Scan(&ct); err != nil {
			t.Fatal(err)
		}
		if ct[0] != 1 {
			t.Fatal("dry run mutated ciphertext")
		}
	}

	// Apply: every seeded row is now at v2 and still decrypts.
	if _, applied, err := rotateColumn(ctx, pool, v2, tgt, true, 100); err != nil || applied < 3 {
		t.Fatalf("apply: applied=%d err=%v", applied, err)
	}
	for _, id := range ids {
		var ct []byte
		if err := pool.QueryRow(ctx, `SELECT email_ciphertext FROM accounts WHERE id = $1`, id).Scan(&ct); err != nil {
			t.Fatal(err)
		}
		if ct[0] != 2 {
			t.Fatalf("row %s not at v2: prefix %d", id, ct[0])
		}
		if got, err := v2.Open(ct); err != nil || got != "user"+id+"@example.test" {
			t.Fatalf("row %s decrypt after rotation: %q %v", id, got, err)
		}
	}

	// A second apply is a no-op: nothing is stale anymore.
	if _, stale, err := rotateColumn(ctx, pool, v2, tgt, true, 100); err != nil || stale != 0 {
		t.Fatalf("second apply: stale=%d err=%v, want 0", stale, err)
	}
}

func TestRotateEmailLookupHash(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	fieldKey := bytes.Repeat([]byte{7}, 32)
	cipher, err := auth.NewFieldCipher(fieldKey)
	if err != nil {
		t.Fatal(err)
	}
	oldLookup := bytes.Repeat([]byte{1}, 32)
	newLookup := bytes.Repeat([]byte{2}, 32)
	legacyHasher, err := auth.NewLookupHasher(oldLookup)
	if err != nil {
		t.Fatal(err)
	}
	rotatedHasher, err := auth.NewLookupHasher(newLookup, oldLookup)
	if err != nil {
		t.Fatal(err)
	}

	ids := make([]string, 3)
	for i := range ids {
		id := dbtest.InsertAccount(t, ctx, pool, "")
		ids[i] = id.String()
		email := "user" + id.String() + "@example.test"
		sealed, err := cipher.Seal(email)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE accounts SET email_ciphertext = $2, email_lookup_hash = $3 WHERE id = $1`,
			id, sealed, legacyHasher.Hash(auth.NormalizeEmail(email))); err != nil {
			t.Fatal(err)
		}
	}

	// Dry run reports the stale rows without touching them.
	scanned, stale, err := rotateEmailLookupHash(ctx, pool, cipher, rotatedHasher, false, 100)
	if err != nil || stale < 3 || scanned < 3 {
		t.Fatalf("dry run: scanned=%d stale=%d err=%v", scanned, stale, err)
	}
	for _, id := range ids {
		var h []byte
		if err := pool.QueryRow(ctx, `SELECT email_lookup_hash FROM accounts WHERE id = $1`, id).Scan(&h); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(h, legacyHasher.Hash(auth.NormalizeEmail("user"+id+"@example.test"))) {
			t.Fatal("dry run mutated email_lookup_hash")
		}
	}

	// Apply rewrites each hash with the primary key; the row stays resolvable.
	if _, applied, err := rotateEmailLookupHash(ctx, pool, cipher, rotatedHasher, true, 100); err != nil || applied < 3 {
		t.Fatalf("apply: applied=%d err=%v", applied, err)
	}
	for _, id := range ids {
		email := "user" + id + "@example.test"
		var stored []byte
		if err := pool.QueryRow(ctx, `SELECT email_lookup_hash FROM accounts WHERE id = $1`, id).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(stored, rotatedHasher.Hash(auth.NormalizeEmail(email))) {
			t.Fatalf("row %s not rewritten with primary key", id)
		}
		if rotatedHasher.NeedsRotation(stored, auth.NormalizeEmail(email)) {
			t.Fatalf("row %s still flagged for rotation", id)
		}
	}

	// Second apply is a no-op.
	if _, stale, err := rotateEmailLookupHash(ctx, pool, cipher, rotatedHasher, true, 100); err != nil || stale != 0 {
		t.Fatalf("second apply: stale=%d err=%v, want 0", stale, err)
	}
}

// account_admin_mfa is keyed by account_id, not id — regression guard for the
// per-target key column.
func TestRotateColumnNonIDKey(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	k1 := bytes.Repeat([]byte{3}, 32)
	k2 := bytes.Repeat([]byte{4}, 32)
	v1, err := auth.NewFieldCipherRing(1, map[byte][]byte{1: k1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	acct := dbtest.InsertAccount(t, ctx, pool, "")
	sealed, err := v1.Seal("TOTPSECRET234567")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO account_admin_mfa (account_id, secret_ciphertext, enabled_at) VALUES ($1, $2, CURRENT_TIMESTAMP)`,
		acct, sealed); err != nil {
		t.Fatal(err)
	}

	v2, err := auth.NewFieldCipherRing(2, map[byte][]byte{1: k1, 2: k2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tgt := target{table: "account_admin_mfa", column: "secret_ciphertext", key: "account_id"}
	if _, applied, err := rotateColumn(ctx, pool, v2, tgt, true, 100); err != nil || applied != 1 {
		t.Fatalf("rotate account_admin_mfa: applied=%d err=%v", applied, err)
	}
	var ct []byte
	if err := pool.QueryRow(ctx, `SELECT secret_ciphertext FROM account_admin_mfa WHERE account_id = $1`, acct).Scan(&ct); err != nil {
		t.Fatal(err)
	}
	if ct[0] != 2 {
		t.Fatalf("secret not rotated: prefix %d", ct[0])
	}
	if got, err := v2.Open(ct); err != nil || got != "TOTPSECRET234567" {
		t.Fatalf("decrypt after rotation: %q %v", got, err)
	}
}
