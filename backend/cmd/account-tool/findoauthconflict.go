package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/auth"
)

// oauthConflictMatch is one other account sharing the source account's email
// address — the candidate for "which account is my Google identity actually
// bound to" when a fresh OAuth bind hits ErrConflict.
type oauthConflictMatch struct {
	AccountID      uuid.UUID `json:"account_id"`
	Username       string    `json:"username"`
	AccountStatus  string    `json:"account_status"`
	HasGoogleOAuth bool      `json:"has_google_oauth"`
}

// findOAuthConflict decrypts id's email, hashes it under every active lookup
// key (primary + any retired secondary — see auth.LookupHasher.Candidates),
// and returns every *other* account whose email_lookup_hash matches one of
// those candidates, each annotated with whether it already holds a Google
// oauth_identities row. It never writes anything.
func findOAuthConflict(ctx context.Context, pool *pgxpool.Pool, cipher *auth.FieldCipher, hasher *auth.LookupHasher, id uuid.UUID) ([]oauthConflictMatch, error) {
	var emailCiphertext []byte
	if err := pool.QueryRow(ctx, `SELECT email_ciphertext FROM accounts WHERE id = $1`, id).Scan(&emailCiphertext); err != nil {
		return nil, fmt.Errorf("load source account email: %w", err)
	}
	plainEmail, err := cipher.Open(emailCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt source account email: %w", err)
	}
	candidates := hasher.Candidates(auth.NormalizeEmail(plainEmail))

	rows, err := pool.Query(ctx, `
		SELECT id, username, account_status
		FROM accounts
		WHERE id != $1 AND email_lookup_hash = ANY($2)
	`, id, candidates)
	if err != nil {
		return nil, fmt.Errorf("query matching accounts: %w", err)
	}
	defer rows.Close()

	matches := make([]oauthConflictMatch, 0)
	for rows.Next() {
		var m oauthConflictMatch
		if err := rows.Scan(&m.AccountID, &m.Username, &m.AccountStatus); err != nil {
			return nil, fmt.Errorf("scan matching account: %w", err)
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range matches {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM oauth_identities WHERE account_id = $1 AND provider = 'google')`,
			matches[i].AccountID).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("check oauth_identities for %s: %w", matches[i].AccountID, err)
		}
		matches[i].HasGoogleOAuth = exists
	}
	return matches, nil
}

// findByEmail hashes plainEmail under every active lookup key and returns
// every account matching it — support's version of "which account is
// registered under this literal address", independent of any other
// account's email (unlike findOAuthConflict, which starts from a known
// account and only checks accounts sharing *that* account's email).
func findByEmail(ctx context.Context, pool *pgxpool.Pool, hasher *auth.LookupHasher, plainEmail string) ([]oauthConflictMatch, error) {
	candidates := hasher.Candidates(auth.NormalizeEmail(plainEmail))

	rows, err := pool.Query(ctx, `
		SELECT id, username, account_status
		FROM accounts
		WHERE email_lookup_hash = ANY($1)
	`, candidates)
	if err != nil {
		return nil, fmt.Errorf("query matching accounts: %w", err)
	}
	defer rows.Close()

	matches := make([]oauthConflictMatch, 0)
	for rows.Next() {
		var m oauthConflictMatch
		if err := rows.Scan(&m.AccountID, &m.Username, &m.AccountStatus); err != nil {
			return nil, fmt.Errorf("scan matching account: %w", err)
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range matches {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM oauth_identities WHERE account_id = $1 AND provider = 'google')`,
			matches[i].AccountID).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("check oauth_identities for %s: %w", matches[i].AccountID, err)
		}
		matches[i].HasGoogleOAuth = exists
	}
	return matches, nil
}
