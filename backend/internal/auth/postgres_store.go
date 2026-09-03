package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is nil")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) CreateAccount(ctx context.Context, username string, emailCiphertext, emailLookupHash []byte, passwordHash string) (Account, error) {
	var idText string
	var account Account
	err := s.pool.QueryRow(ctx, `
		INSERT INTO accounts (username, email_ciphertext, email_lookup_hash, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, username, identity_status, account_status, email_verified_at IS NOT NULL
	`, username, emailCiphertext, emailLookupHash, passwordHash).Scan(
		&idText, &account.Username, &account.IdentityStatus, &account.AccountStatus, &account.EmailVerified,
	)
	if err != nil {
		return Account{}, mapStoreError(err)
	}
	account.ID, err = uuid.Parse(idText)
	if err != nil {
		return Account{}, fmt.Errorf("parse created account id: %w", err)
	}
	return account, nil
}

func (s *PostgresStore) FindAccountByUsername(ctx context.Context, username string) (Account, string, error) {
	var idText string
	var account Account
	var passwordHash string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, username, identity_status, account_status, email_verified_at IS NOT NULL, password_hash
		FROM accounts
		WHERE username = $1
	`, username).Scan(&idText, &account.Username, &account.IdentityStatus, &account.AccountStatus, &account.EmailVerified, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, "", ErrNotFound
	}
	if err != nil {
		return Account{}, "", fmt.Errorf("find account by username: %w", err)
	}
	account.ID, err = uuid.Parse(idText)
	if err != nil {
		return Account{}, "", fmt.Errorf("parse account id: %w", err)
	}
	return account, passwordHash, nil
}

func (s *PostgresStore) FindAccountByID(ctx context.Context, accountID uuid.UUID) (Account, error) {
	var idText string
	var account Account
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, username, identity_status, account_status, email_verified_at IS NOT NULL
		FROM accounts
		WHERE id = $1
	`, accountID).Scan(&idText, &account.Username, &account.IdentityStatus, &account.AccountStatus, &account.EmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("find account by id: %w", err)
	}
	account.ID, err = uuid.Parse(idText)
	if err != nil {
		return Account{}, fmt.Errorf("parse account id: %w", err)
	}
	if account.AccountStatus != "active" {
		return Account{}, ErrNotFound
	}
	return account, nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, accountID uuid.UUID, tokenHash, csrfHash []byte, expiresAt time.Time, ipHash, userAgentHash []byte) (uuid.UUID, error) {
	var idText string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO account_sessions
			(account_id, token_hash, csrf_token_hash, expires_at, ip_hash, user_agent_hash)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text
	`, accountID, tokenHash, csrfHash, expiresAt, ipHash, userAgentHash).Scan(&idText)
	if err != nil {
		return uuid.Nil, mapStoreError(err)
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse created session id: %w", err)
	}
	return id, nil
}

func (s *PostgresStore) FindActiveSession(ctx context.Context, tokenHash []byte, now time.Time) (Session, error) {
	var session Session
	var sessionIDText, accountIDText string
	err := s.pool.QueryRow(ctx, `
		SELECT s.id::text, s.account_id::text, s.csrf_token_hash, s.expires_at,
		       a.username, a.identity_status, a.account_status, a.email_verified_at IS NOT NULL
		FROM account_sessions s
		JOIN accounts a ON a.id = s.account_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > $2
	`, tokenHash, now).Scan(
		&sessionIDText, &accountIDText, &session.CSRFHash, &session.ExpiresAt,
		&session.Account.Username, &session.Account.IdentityStatus, &session.Account.AccountStatus, &session.Account.EmailVerified,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("find active session: %w", err)
	}
	session.ID, err = uuid.Parse(sessionIDText)
	if err != nil {
		return Session{}, fmt.Errorf("parse session id: %w", err)
	}
	session.Account.ID, err = uuid.Parse(accountIDText)
	if err != nil {
		return Session{}, fmt.Errorf("parse session account id: %w", err)
	}
	if session.Account.AccountStatus != "active" {
		return Session{}, ErrNotFound
	}
	return session, nil
}

func (s *PostgresStore) TouchSession(ctx context.Context, sessionID uuid.UUID, now time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE account_sessions SET last_seen_at = $2 WHERE id = $1`, sessionID, now)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (s *PostgresStore) RevokeSession(ctx context.Context, sessionID uuid.UUID, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE account_sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE id = $1
	`, sessionID, now)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *PostgresStore) CreateOAuthBinding(ctx context.Context, accountID uuid.UUID, provider string, providerSubjectHash []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_identities (account_id, provider, provider_subject_hash)
		VALUES ($1, $2, $3)
	`, accountID, provider, providerSubjectHash)
	if err != nil {
		return mapStoreError(err)
	}
	return nil
}

func (s *PostgresStore) FindAccountByOAuthSubject(ctx context.Context, provider string, providerSubjectHash []byte) (Account, error) {
	var idText string
	var account Account
	err := s.pool.QueryRow(ctx, `
		SELECT a.id::text, a.username, a.identity_status, a.account_status, a.email_verified_at IS NOT NULL
		FROM oauth_identities o
		JOIN accounts a ON a.id = o.account_id
		WHERE o.provider = $1 AND o.provider_subject_hash = $2
	`, provider, providerSubjectHash).Scan(
		&idText, &account.Username, &account.IdentityStatus, &account.AccountStatus, &account.EmailVerified,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("find OAuth account: %w", err)
	}
	account.ID, err = uuid.Parse(idText)
	if err != nil {
		return Account{}, fmt.Errorf("parse OAuth account id: %w", err)
	}
	if account.AccountStatus != "active" {
		return Account{}, ErrNotFound
	}
	return account, nil
}

func (s *PostgresStore) CreateOAuthState(ctx context.Context, provider string, accountID *uuid.UUID, stateHash, codeVerifierCiphertext []byte, redirectURL string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_states
			(provider, account_id, state_hash, code_verifier_ciphertext, redirect_url, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, provider, accountID, stateHash, codeVerifierCiphertext, redirectURL, expiresAt)
	if err != nil {
		return mapStoreError(err)
	}
	return nil
}

func (s *PostgresStore) ConsumeOAuthState(ctx context.Context, provider string, stateHash []byte, now time.Time) (OAuthState, error) {
	var accountIDText string
	var state OAuthState
	err := s.pool.QueryRow(ctx, `
		UPDATE oauth_states
		SET consumed_at = $3
		WHERE provider = $1
		  AND state_hash = $2
		  AND consumed_at IS NULL
		  AND expires_at > $3
		RETURNING COALESCE(account_id::text, ''), code_verifier_ciphertext, redirect_url
	`, provider, stateHash, now).Scan(&accountIDText, &state.CodeVerifierCiphertext, &state.RedirectURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return OAuthState{}, ErrExpired
	}
	if err != nil {
		return OAuthState{}, fmt.Errorf("consume OAuth state: %w", err)
	}
	state.Provider = provider
	if accountIDText != "" {
		accountID, err := uuid.Parse(accountIDText)
		if err != nil {
			return OAuthState{}, fmt.Errorf("parse OAuth state account id: %w", err)
		}
		state.AccountID = &accountID
	}
	return state, nil
}

func (s *PostgresStore) CreateEmailVerificationChallenge(ctx context.Context, accountID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE email_verification_challenges SET consumed_at = CURRENT_TIMESTAMP WHERE account_id = $1 AND consumed_at IS NULL`, accountID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO email_verification_challenges (account_id, token_hash, expires_at) VALUES ($1, $2, $3)`, accountID, tokenHash, expiresAt); err != nil {
		return mapStoreError(err)
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ConsumeEmailVerificationChallenge(ctx context.Context, accountID uuid.UUID, tokenHash []byte, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var challengeID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM email_verification_challenges
		WHERE account_id = $1 AND token_hash = $2 AND consumed_at IS NULL AND expires_at > $3
		ORDER BY created_at DESC LIMIT 1 FOR UPDATE
	`, accountID, tokenHash, now).Scan(&challengeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExpired
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE email_verification_challenges SET consumed_at = $2 WHERE id = $1`, challengeID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE accounts SET email_verified_at = COALESCE(email_verified_at, $2), updated_at = CURRENT_TIMESTAMP WHERE id = $1 AND account_status = 'active'`, accountID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ConsumeEmailVerificationToken(ctx context.Context, tokenHash []byte, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var challengeID, accountID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id, account_id FROM email_verification_challenges
		WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		ORDER BY created_at DESC LIMIT 1 FOR UPDATE
	`, tokenHash, now).Scan(&challengeID, &accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExpired
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE email_verification_challenges SET consumed_at = $2 WHERE id = $1`, challengeID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE accounts SET email_verified_at = COALESCE(email_verified_at, $2), updated_at = CURRENT_TIMESTAMP WHERE id = $1 AND account_status = 'active'`, accountID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- session management --------------------------------------------------

func (s *PostgresStore) ListActiveSessions(ctx context.Context, accountID uuid.UUID, now time.Time) ([]SessionSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, created_at, last_seen_at, expires_at
		FROM account_sessions
		WHERE account_id = $1 AND revoked_at IS NULL AND expires_at > $2
		ORDER BY COALESCE(last_seen_at, created_at) DESC
	`, accountID, now)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	result := make([]SessionSummary, 0)
	for rows.Next() {
		var item SessionSummary
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.LastSeenAt, &item.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *PostgresStore) RevokeAccountSession(ctx context.Context, accountID, sessionID uuid.UUID, now time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE account_sessions SET revoked_at = COALESCE(revoked_at, $3)
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL
	`, sessionID, accountID, now)
	if err != nil {
		return false, fmt.Errorf("revoke session: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PostgresStore) RevokeOtherAccountSessions(ctx context.Context, accountID, keepSessionID uuid.UUID, now time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE account_sessions SET revoked_at = COALESCE(revoked_at, $3)
		WHERE account_id = $1 AND id <> $2 AND revoked_at IS NULL
	`, accountID, keepSessionID, now)
	if err != nil {
		return 0, fmt.Errorf("revoke other sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// --- password reset -------------------------------------------------------

func (s *PostgresStore) LookupAccountIDByEmailHash(ctx context.Context, emailLookupHash []byte) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM accounts WHERE email_lookup_hash = $1 AND account_status = 'active'
	`, emailLookupHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup account by email hash: %w", err)
	}
	return id, nil
}

func (s *PostgresStore) CreatePasswordResetChallenge(ctx context.Context, accountID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE password_reset_challenges SET consumed_at = CURRENT_TIMESTAMP
		WHERE account_id = $1 AND consumed_at IS NULL
	`, accountID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO password_reset_challenges (account_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, accountID, tokenHash, expiresAt); err != nil {
		return mapStoreError(err)
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ConsumePasswordResetChallenge(ctx context.Context, tokenHash []byte, newPasswordHash string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var challengeID, accountID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id, account_id FROM password_reset_challenges
		WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		ORDER BY created_at DESC LIMIT 1 FOR UPDATE
	`, tokenHash, now).Scan(&challengeID, &accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExpired
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE password_reset_challenges SET consumed_at = $2 WHERE id = $1`, challengeID, now); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE accounts SET password_hash = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND account_status = 'active'
	`, accountID, newPasswordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	// A password reset ends every existing session.
	if _, err := tx.Exec(ctx, `
		UPDATE account_sessions SET revoked_at = COALESCE(revoked_at, $2) WHERE account_id = $1
	`, accountID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) UpdatePasswordForAccount(ctx context.Context, accountID uuid.UUID, newPasswordHash string, keepSessionID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE accounts SET password_hash = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND account_status = 'active'
	`, accountID, newPasswordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE account_sessions SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
		WHERE account_id = $1 AND id <> $2
	`, accountID, keepSessionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin'
		)
	`, accountID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check administrator role: %w", err)
	}
	return exists, nil
}

func (s *PostgresStore) GetAdminMFA(ctx context.Context, accountID uuid.UUID) (AdminMFARecord, error) {
	var record AdminMFARecord
	err := s.pool.QueryRow(ctx, `
		SELECT secret_ciphertext, enabled_at, pending_expires_at
		FROM account_admin_mfa
		WHERE account_id = $1
	`, accountID).Scan(&record.SecretCiphertext, &record.EnabledAt, &record.PendingExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminMFARecord{}, ErrNotFound
	}
	if err != nil {
		return AdminMFARecord{}, fmt.Errorf("get administrator MFA: %w", err)
	}
	return record, nil
}

func (s *PostgresStore) SaveAdminMFASecret(ctx context.Context, accountID uuid.UUID, ciphertext []byte, expiresAt time.Time) error {
	if len(ciphertext) == 0 {
		return ErrInvalidInput
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO account_admin_mfa (account_id, secret_ciphertext, pending_expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (account_id) DO UPDATE
		SET secret_ciphertext = EXCLUDED.secret_ciphertext,
		    enabled_at = NULL,
		    pending_expires_at = EXCLUDED.pending_expires_at,
		    updated_at = CURRENT_TIMESTAMP
		WHERE account_admin_mfa.enabled_at IS NULL
	`, accountID, ciphertext, expiresAt)
	if err != nil {
		return mapStoreError(err)
	}
	return nil
}

func (s *PostgresStore) EnableAdminMFA(ctx context.Context, accountID uuid.UUID, enabledAt time.Time) error {
	command, err := s.pool.Exec(ctx, `
		UPDATE account_admin_mfa
		SET enabled_at = $2, pending_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE account_id = $1 AND enabled_at IS NULL
	`, accountID, enabledAt)
	if err != nil {
		return fmt.Errorf("enable administrator MFA: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (s *PostgresStore) DisableAdminMFA(ctx context.Context, accountID uuid.UUID) error {
	command, err := s.pool.Exec(ctx, `
		DELETE FROM account_admin_mfa WHERE account_id = $1
	`, accountID)
	if err != nil {
		return fmt.Errorf("disable administrator MFA: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func mapStoreError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
