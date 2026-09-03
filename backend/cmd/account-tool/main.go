// Command account-tool exports or erases one account's personal data.
//
//	account-tool -mode export -account <uuid|username> [-out file.json]
//	account-tool -mode erase  -account <uuid|username> -reason "..." -yes
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
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/auth"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mode := flag.String("mode", "", "export | erase")
	account := flag.String("account", "", "account UUID or username")
	out := flag.String("out", "", "export: write JSON here instead of stdout")
	reason := flag.String("reason", "", "erase: audit reason (required)")
	confirm := flag.Bool("yes", false, "erase: required to actually apply changes")
	flag.Parse()

	if err := run(logger, *mode, *account, *out, *reason, *confirm); err != nil {
		logger.Error("account-tool failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, mode, account, out, reason string, confirm bool) error {
	if mode != "export" && mode != "erase" {
		return errors.New("-mode must be export or erase")
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
	pool, err := db.OpenPostgres(ctx, cfg.DatabaseURL)
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

func randomTombstone() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "erased-" + hex.EncodeToString(b)
}
