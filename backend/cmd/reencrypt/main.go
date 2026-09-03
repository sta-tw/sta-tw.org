// Command reencrypt rewrites every column-level PII ciphertext with the current
// primary field-encryption key. Run it after adding a new key version to
// STA_FIELD_ENCRYPTION_KEYS and pointing STA_FIELD_ENCRYPTION_PRIMARY_VERSION at
// it: the API can already read both, this migrates the data at rest so the old
// key can eventually be retired.
//
//	reencrypt            # dry run: report how many rows need rewriting
//	reencrypt -apply     # rewrite them
//
// It only touches AES-GCM ciphertext. HMAC lookup hashes (email_lookup_hash,
// candidate_number_lookup_hash, school_email_lookup_hash) are derived from
// normalised plaintext and are not rotated here.
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
}

var targets = []target{
	{"accounts", "email_ciphertext"},
	{"notifications", "title_ciphertext"},
	{"notifications", "body_ciphertext"},
	{"email_outbox", "recipient_ciphertext"},
	{"email_outbox", "payload_ciphertext"},
	{"account_admin_mfa", "secret_ciphertext"},
	{"oauth_states", "code_verifier_ciphertext"},
	{"applications", "candidate_number_ciphertext"},
	{"verification_requests", "school_email_ciphertext"},
	{"support_tickets", "requester_email_ciphertext"},
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
	logger.Info("reencrypt complete", "scanned", totalScanned, "stale", totalRewritten, "apply", apply)
	if !apply && totalRewritten > 0 {
		logger.Warn("dry run — re-run with -apply to rewrite", "stale_rows", totalRewritten)
	}
	return nil
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
	selectSQL := fmt.Sprintf(
		`SELECT id::text, %s FROM %s WHERE %s IS NOT NULL AND ($1 = '' OR id > $1::uuid) ORDER BY id LIMIT $2`,
		t.column, t.table, t.column)
	updateSQL := fmt.Sprintf(`UPDATE %s SET %s = $2 WHERE id = $1::uuid`, t.table, t.column)

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
