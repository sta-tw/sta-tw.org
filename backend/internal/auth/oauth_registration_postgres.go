package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

func (s *PostgresStore) CreateOAuthAccount(ctx context.Context, data oauthAccountData) (Account, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM oauth_registration_consumptions WHERE expires_at < CURRENT_TIMESTAMP`); err != nil {
		return Account{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO oauth_registration_consumptions (nonce_hash,expires_at) VALUES ($1,$2)`, data.NonceHash, data.RegistrationExpires); err != nil {
		return Account{}, mapStoreError(err)
	}
	// Serialize all registrations and binds for this provider/subject; the unique
	// constraint remains the final arbiter for concurrent inserts.
	if len(data.SubjectHashes) == 0 {
		return Account{}, ErrInvalidInput
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM oauth_identities WHERE provider=$1 AND provider_subject_hash=ANY($2))`, data.Provider, data.SubjectHashes).Scan(&exists); err != nil {
		return Account{}, err
	}
	if exists {
		return Account{}, ErrConflict
	}
	status, identity := "pending_verification", "temporary"
	if data.RoleVerified {
		status, identity = "active", "student"
	}
	var a Account
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO accounts (username,email_ciphertext,email_lookup_hash,school_email_ciphertext,school_email_lookup_hash,password_hash,account_status,identity_status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,username,identity_status,account_status,email_verified_at IS NOT NULL`, data.Username, data.EmailCiphertext, data.EmailHash, data.SchoolCiphertext, data.SchoolHash, data.PasswordHash, status, identity).Scan(&id, &a.Username, &a.IdentityStatus, &a.AccountStatus, &a.EmailVerified)
	if err != nil {
		return Account{}, mapStoreError(err)
	}
	a.ID, err = uuid.Parse(id)
	if err != nil {
		return Account{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO oauth_identities (account_id,provider,provider_subject_hash) VALUES ($1,$2,$3)`, a.ID, data.Provider, data.SubjectHashes[0])
	if err != nil {
		return Account{}, mapStoreError(err)
	}
	if data.RoleVerified {
		_, err = tx.Exec(ctx, `INSERT INTO oauth_registration_verifications (account_id,provider,guild_id,role_id) VALUES ($1,'discord',$2,$3)`, a.ID, discordGuild, discordRole)
		if err != nil {
			return Account{}, err
		}
	} else {
		if len(data.Payload) == 0 || len(data.ChallengeHash) == 0 {
			return Account{}, errors.New("missing registration challenge")
		}
		_, err = tx.Exec(ctx, `INSERT INTO password_reset_challenges (account_id,token_hash,expires_at,activates_account) VALUES ($1,$2,$3,true)`, a.ID, data.ChallengeHash, data.Expires)
		if err != nil {
			return Account{}, mapStoreError(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO email_outbox (account_id,dedup_key,recipient_ciphertext,payload_ciphertext) VALUES ($1,$2,$3,$4)`, a.ID, "oauth-register:"+a.ID.String(), data.SchoolCiphertext, data.Payload)
		if err != nil {
			return Account{}, fmt.Errorf("enqueue activation: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Account{}, mapStoreError(err)
	}
	return a, nil
}
