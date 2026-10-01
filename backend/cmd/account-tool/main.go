// Command account-tool exports or erases one account's personal data.
//
//	account-tool -mode export -account <uuid|username> [-out file.json]
//	account-tool -mode erase  -account <uuid|username> -reason "..." -yes
//	account-tool -mode resend-activation -account <uuid|username>
//
// export is read-only: it decrypts and dumps everything the platform holds for
// the account as JSON. erase runs one transaction that revokes all access,
// deletes credential/transient rows, scrubs direct-PII columns everywhere they
// are nullable (or replaces them with a tombstone where a NOT NULL / UNIQUE /
// CHECK constraint forbids NULL), de-identifies authored content, and sets
// account_status = 'deleted'. Because every FK into accounts is ON DELETE
// RESTRICT, the account row itself is kept — anonymised, not removed.
//
// erase does NOT delete objects from object storage. It prints every storage
// key it touched (portfolio files, verification documents, support
// attachments); delete those out of band.
//
// resend-activation re-sends the "STA 帳號啟用" school-email link for an
// account still stuck in 'pending_verification' (e.g. the original send
// predates a mail-deliverability fix and never reached the inbox). It reuses
// the original recipient_ciphertext from the account's earlier
// register-school-email email_outbox row — the school address is never
// decrypted back to plaintext, just carried over byte-for-byte — and issues a
// fresh 24h activation token the same way auth.Service.Register does.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/auth"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
	"sta-backend/internal/email"
	"sta-backend/internal/notifications"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mode := flag.String("mode", "", "export | erase | resend-activation | find-oauth-conflict | find-by-email")
	account := flag.String("account", "", "account UUID or username")
	email := flag.String("email", "", "find-by-email: the plaintext email to look up")
	out := flag.String("out", "", "export: write JSON here instead of stdout")
	reason := flag.String("reason", "", "erase: audit reason (required)")
	confirm := flag.Bool("yes", false, "erase: required to actually apply changes")
	flag.Parse()

	if err := run(logger, *mode, *account, *email, *out, *reason, *confirm); err != nil {
		logger.Error("account-tool failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, mode, account, email, out, reason string, confirm bool) error {
	if mode != "export" && mode != "erase" && mode != "resend-activation" && mode != "find-oauth-conflict" && mode != "find-by-email" {
		return errors.New("-mode must be export, erase, resend-activation, find-oauth-conflict, or find-by-email")
	}
	if mode == "find-by-email" {
		if strings.TrimSpace(email) == "" {
			return errors.New("-email is required for find-by-email")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.DatabaseURL == "" {
			return errors.New("STA_DATABASE_URL is required")
		}
		hasher, err := auth.NewLookupHasher(cfg.LookupHMACKey, cfg.LookupHMACSecondaryKeys...)
		if err != nil {
			return fmt.Errorf("lookup hasher: %w", err)
		}
		if hasher == nil {
			return errors.New("STA_LOOKUP_HMAC_KEY is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		pool, err := db.OpenPostgres(ctx, cfg.DatabaseURL, 5)
		if err != nil {
			return err
		}
		defer pool.Close()
		matches, err := findByEmail(ctx, pool, hasher, email)
		if err != nil {
			return err
		}
		encoded, _ := json.MarshalIndent(matches, "", "  ")
		fmt.Println(string(encoded))
		return nil
	}
	if strings.TrimSpace(account) == "" {
		return errors.New("-account is required")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required")
	}
	cipher, err := auth.NewFieldCipher(cfg.EmailEncryptionKey)
	if err != nil {
		return fmt.Errorf("field cipher: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := db.OpenPostgres(ctx, cfg.DatabaseURL, 5)
	if err != nil {
		return err
	}
	defer pool.Close()

	id, username, status, err := resolveAccount(ctx, pool, account)
	if err != nil {
		return err
	}
	logger.Info("account resolved", "id", id, "username", username, "status", status)

	switch mode {
	case "find-oauth-conflict":
		hasher, err := auth.NewLookupHasher(cfg.LookupHMACKey, cfg.LookupHMACSecondaryKeys...)
		if err != nil {
			return fmt.Errorf("lookup hasher: %w", err)
		}
		if hasher == nil {
			return errors.New("STA_LOOKUP_HMAC_KEY is required")
		}
		matches, err := findOAuthConflict(ctx, pool, cipher, hasher, id)
		if err != nil {
			return err
		}
		encoded, _ := json.MarshalIndent(matches, "", "  ")
		fmt.Println(string(encoded))
		return nil
	case "resend-activation":
		if status != "pending_verification" {
			return fmt.Errorf("account status is %q, not pending_verification — nothing to activate", status)
		}
		if err := resendActivation(ctx, pool, cipher, cfg.PublicBaseURL, id); err != nil {
			return err
		}
		logger.Info("activation email re-queued", "id", id, "username", username)
		return nil
	case "export":
		bundle, err := exportAccount(ctx, pool, cipher, id)
		if err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(bundle, "", "  ")
		if err != nil {
			return err
		}
		if out == "" {
			fmt.Println(string(encoded))
			return nil
		}
		if err := os.WriteFile(out, append(encoded, '\n'), 0o600); err != nil {
			return err
		}
		logger.Info("export written", "path", out, "bytes", len(encoded))
		return nil
	default: // erase
		if strings.TrimSpace(reason) == "" {
			return errors.New("-reason is required for erase")
		}
		if status == "deleted" {
			return errors.New("account is already deleted")
		}
		report, err := eraseAccount(ctx, pool, id, reason, confirm)
		if err != nil {
			return err
		}
		encoded, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(encoded))
		if !confirm {
			logger.Warn("dry run — pass -yes to apply")
		} else {
			logger.Info("account erased", "id", id)
		}
		return nil
	}
}

func resolveAccount(ctx context.Context, pool *pgxpool.Pool, account string) (uuid.UUID, string, string, error) {
	var (
		id       uuid.UUID
		username string
		status   string
		row      pgx.Row
	)
	if parsed, err := uuid.Parse(strings.TrimSpace(account)); err == nil {
		row = pool.QueryRow(ctx, `SELECT id, username, account_status FROM accounts WHERE id = $1`, parsed)
	} else {
		row = pool.QueryRow(ctx, `SELECT id, username, account_status FROM accounts WHERE username = $1`,
			strings.ToLower(strings.TrimSpace(account)))
	}
	if err := row.Scan(&id, &username, &status); errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", "", errors.New("account not found")
	} else if err != nil {
		return uuid.Nil, "", "", err
	}
	return id, username, status, nil
}

// resendActivation issues a fresh activation token for a still-pending
// account and re-queues the "STA 帳號啟用" email to whatever school address
// the original registration used, without ever decrypting that address: the
// recipient_ciphertext blob from the earlier register-school-email row is
// carried over as-is (same field cipher, same key — no re-encryption needed).
func resendActivation(ctx context.Context, pool *pgxpool.Pool, cipher *auth.FieldCipher, publicBaseURL string, accountID uuid.UUID) error {
	var recipientCiphertext []byte
	err := pool.QueryRow(ctx, `
		SELECT recipient_ciphertext FROM email_outbox
		WHERE account_id = $1 AND dedup_key LIKE 'register-school-email:%'
		ORDER BY created_at DESC LIMIT 1
	`, accountID).Scan(&recipientCiphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("no prior register-school-email outbox row found for this account")
	}
	if err != nil {
		return err
	}

	token, err := auth.NewOpaqueToken(32)
	if err != nil {
		return err
	}
	tokenHash := auth.HashOpaqueToken(token)
	expiresAt := time.Now().UTC().Add(24 * time.Hour)

	tx, err := pool.Begin(ctx)
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
		INSERT INTO password_reset_challenges (account_id, token_hash, expires_at, activates_account)
		VALUES ($1, $2, $3, true)
	`, accountID, tokenHash, expiresAt); err != nil {
		return err
	}

	link := publicBaseURL + "/reset-password?token=" + url.QueryEscape(token)
	textBody := "歡迎加入 STA！請在 24 小時內點擊以下連結，設定密碼以啟用帳號：\n" + link
	logoURL := ""
	if publicBaseURL != "" {
		logoURL = publicBaseURL + "/logo.svg"
	}
	htmlBody := email.AccountActivationEmail(email.AccountActivationEmailData{LogoURL: logoURL, SetURL: link})
	payload, err := json.Marshal(notifications.EmailPayload{Subject: "[特殊選才資源網] 帳號啟用信", Text: textBody, HTML: htmlBody})
	if err != nil {
		return err
	}
	payloadCiphertext, err := cipher.Seal(string(payload))
	if err != nil {
		return err
	}
	dedupKey := "register-school-email:" + accountID.String() + ":" + hex.EncodeToString(tokenHash)
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_outbox (account_id, dedup_key, recipient_ciphertext, payload_ciphertext)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id, dedup_key) DO NOTHING
	`, accountID, dedupKey, recipientCiphertext, payloadCiphertext); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func randomTombstone() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "erased-" + hex.EncodeToString(b)
}
