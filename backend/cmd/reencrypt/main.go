// Command reencrypt rewrites every column-level PII ciphertext with the current
// primary field-encryption key. Run it after adding a new key version to
// STA_FIELD_ENCRYPTION_KEYS and pointing STA_FIELD_ENCRYPTION_PRIMARY_VERSION at
// it: the API can already read both, this migrates the data at rest so the old
// key can eventually be retired.
//
//	reencrypt            # dry run: report how many rows need rewriting
//	reencrypt -apply     # rewrite them
//
// It rewrites AES-GCM ciphertext for every key version, and — when
// STA_LOOKUP_HMAC_SECONDARY_KEYS is set — also recomputes accounts.email_lookup_hash
// with the primary lookup key, since that hash's plaintext (the email) is
// recoverable from email_ciphertext. Other HMAC lookup hashes
// (candidate_number_lookup_hash, school_email_lookup_hash) are left untouched.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/auth"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
)

// target is one ciphertext column keyed by the table's uuid primary key.
type target struct {
	table  string
	column string
	key    string // primary key column to page by (uuid); defaults to "id"
}

func (t target) keyColumn() string {
	if t.key == "" {
		return "id"
	}
	return t.key
}

var targets = []target{
	{table: "accounts", column: "email_ciphertext"},
	{table: "notifications", column: "title_ciphertext"},
	{table: "notifications", column: "body_ciphertext"},
	{table: "email_outbox", column: "recipient_ciphertext"},
	{table: "email_outbox", column: "payload_ciphertext"},
	{table: "account_admin_mfa", column: "secret_ciphertext", key: "account_id"},
	{table: "oauth_states", column: "code_verifier_ciphertext"},
	{table: "applications", column: "candidate_number_ciphertext"},
	{table: "verification_requests", column: "school_email_ciphertext"},
	{table: "support_tickets", column: "requester_email_ciphertext"},
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	apply := flag.Bool("apply", false, "rewrite rows (default: dry run)")
	batch := flag.Int("batch", 500, "rows per batch")
	flag.Parse()
	if err := run(logger, *apply, *batch); err != nil {
		logger.Error("reencrypt failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, apply bool, batch int) error {
	if batch < 1 || batch > 5000 {
		batch = 500
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required")
	}
	cipher, err := buildCipher(cfg)
	if err != nil {
		return err
	}
	logger.Info("reencrypt starting", "primary_key_version", cipher.PrimaryVersion(), "apply", apply)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	pool, err := db.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	var totalScanned, totalRewritten int
	for _, t := range targets {
		scanned, rewritten, err := rotateColumn(ctx, pool, cipher, t, apply, batch)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", t.table, t.column, err)
		}
		totalScanned += scanned
		totalRewritten += rewritten
		logger.Info("column done", "table", t.table, "column", t.column, "scanned", scanned, "stale", rewritten)
	}
	if len(cfg.LookupHMACSecondaryKeys) > 0 {
		hasher, herr := auth.NewLookupHasher(cfg.LookupHMACKey, cfg.LookupHMACSecondaryKeys...)
		if herr != nil {
			return herr
		}
		scanned, rewritten, lerr := rotateEmailLookupHash(ctx, pool, cipher, hasher, apply, batch)
		if lerr != nil {
			return fmt.Errorf("accounts.email_lookup_hash: %w", lerr)
		}
		totalScanned += scanned
		totalRewritten += rewritten
		logger.Info("column done", "table", "accounts", "column", "email_lookup_hash", "scanned", scanned, "stale", rewritten)
	}

	logger.Info("reencrypt complete", "scanned", totalScanned, "stale", totalRewritten, "apply", apply)
	if !apply && totalRewritten > 0 {
		logger.Warn("dry run — re-run with -apply to rewrite", "stale_rows", totalRewritten)
	}
	return nil
}

// rotateEmailLookupHash walks accounts in id order, decrypts email_ciphertext,
// and rewrites email_lookup_hash for any row whose hash was produced by a
// retired lookup key.
func rotateEmailLookupHash(ctx context.Context, pool *pgxpool.Pool, cipher *auth.FieldCipher, hasher *auth.LookupHasher, apply bool, batch int) (scanned, rewritten int, err error) {
	var afterID string
	const selectSQL = `SELECT id::text, email_ciphertext, email_lookup_hash FROM accounts
	                   WHERE $1 = '' OR id > $1::uuid ORDER BY id LIMIT $2`
	const updateSQL = `UPDATE accounts SET email_lookup_hash = $2 WHERE id = $1::uuid`

	for {
		rows, qErr := pool.Query(ctx, selectSQL, afterID, batch)
		if qErr != nil {
			return scanned, rewritten, qErr
		}
		type staleRow struct {
			id      string
			newHash []byte
		}
		var stale []staleRow
		var lastID string
		n := 0
		for rows.Next() {
			var id string
			var ciphertext, storedHash []byte
			if scanErr := rows.Scan(&id, &ciphertext, &storedHash); scanErr != nil {
				rows.Close()
				return scanned, rewritten, scanErr
			}
			n++
			lastID = id
			email, openErr := cipher.Open(ciphertext)
			if openErr != nil {
				rows.Close()
				return scanned, rewritten, fmt.Errorf("row %s email cannot be decrypted: %w", id, openErr)
			}
			normalized := auth.NormalizeEmail(email)
			if !hasher.NeedsRotation(storedHash, normalized) {
				continue
			}
			stale = append(stale, staleRow{id: id, newHash: hasher.Hash(normalized)})
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return scanned, rewritten, rowsErr
		}
		rows.Close()
		scanned += n

		if apply && len(stale) > 0 {
			tx, txErr := pool.Begin(ctx)
			if txErr != nil {
				return scanned, rewritten, txErr
			}
			for _, s := range stale {
				if _, execErr := tx.Exec(ctx, updateSQL, s.id, s.newHash); execErr != nil {
					_ = tx.Rollback(ctx)
					return scanned, rewritten, execErr
				}
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return scanned, rewritten, commitErr
			}
		}
		rewritten += len(stale)

		if n < batch {
			break
		}
		afterID = lastID
		if ctx.Err() != nil {
			return scanned, rewritten, ctx.Err()
		}
	}
	return scanned, rewritten, nil
}

func buildCipher(cfg config.Config) (*auth.FieldCipher, error) {
	if cfg.FieldEncryptionKeys != nil {
		return auth.NewFieldCipherRing(cfg.FieldEncryptionPrimaryVersion, cfg.FieldEncryptionKeys, cfg.EmailEncryptionKey)
	}
	if len(cfg.EmailEncryptionKey) != 32 {
		return nil, errors.New("STA_EMAIL_ENCRYPTION_KEY (or a key ring) is required")
	}
	return auth.NewFieldCipher(cfg.EmailEncryptionKey)
}

// rotateColumn walks one column in id order. It reads (id, value), and for any
// value not already at the primary version decrypts then re-encrypts it.
func rotateColumn(ctx context.Context, pool *pgxpool.Pool, cipher *auth.FieldCipher, t target, apply bool, batch int) (scanned, rewritten int, err error) {
	var afterID string // uuid text; "" = from the start
	key := t.keyColumn()
	selectSQL := fmt.Sprintf(
		`SELECT %s::text, %s FROM %s WHERE %s IS NOT NULL AND ($1 = '' OR %s > $1::uuid) ORDER BY %s LIMIT $2`,
		key, t.column, t.table, t.column, key, key)
	updateSQL := fmt.Sprintf(`UPDATE %s SET %s = $2 WHERE %s = $1::uuid`, t.table, t.column, key)

	for {
		rows, qErr := pool.Query(ctx, selectSQL, afterID, batch)
		if qErr != nil {
			return scanned, rewritten, qErr
		}
		type staleRow struct {
			id        string
			plaintext string
		}
		var stale []staleRow
		var lastID string
		n := 0
		for rows.Next() {
			var id string
			var ciphertext []byte
			if scanErr := rows.Scan(&id, &ciphertext); scanErr != nil {
				rows.Close()
				return scanned, rewritten, scanErr
			}
			n++
			lastID = id
			if !cipher.NeedsRotation(ciphertext) {
				continue
			}
			plaintext, openErr := cipher.Open(ciphertext)
			if openErr != nil {
				rows.Close()
				return scanned, rewritten, fmt.Errorf("row %s cannot be decrypted with any configured key: %w", id, openErr)
			}
			stale = append(stale, staleRow{id: id, plaintext: plaintext})
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return scanned, rewritten, rowsErr
		}
		rows.Close()
		scanned += n

		if apply && len(stale) > 0 {
			tx, txErr := pool.Begin(ctx)
			if txErr != nil {
				return scanned, rewritten, txErr
			}
			for _, s := range stale {
				sealed, sealErr := cipher.Seal(s.plaintext)
				if sealErr != nil {
					_ = tx.Rollback(ctx)
					return scanned, rewritten, sealErr
				}
				if _, execErr := tx.Exec(ctx, updateSQL, s.id, sealed); execErr != nil {
					_ = tx.Rollback(ctx)
					return scanned, rewritten, execErr
				}
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return scanned, rewritten, commitErr
			}
		}
		rewritten += len(stale)

		if n < batch {
			break
		}
		afterID = lastID
		if ctx.Err() != nil {
			return scanned, rewritten, ctx.Err()
		}
	}
	return scanned, rewritten, nil
}
