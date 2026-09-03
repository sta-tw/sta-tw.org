// Package migrate is the forward-only SQL migration runner used by cmd/migrate
// and by integration-test harnesses that need to build a schema from scratch.
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/config"
	"sta-backend/internal/db"
)

// advisoryLockKey serializes concurrent runners against one database.
const advisoryLockKey = 77415301

// Migration is a single numbered SQL file.
type Migration struct {
	Version      int64
	Name         string
	Path         string
	SQL          []byte
	SHA256       string
	TelegramOnly bool
}

// Options controls how Apply/Run behave.
type Options struct {
	// IncludeTelegram applies the detached Telegram adapter migrations
	// (000025_telegram_*, 000026_telegram_*), which are skipped by default.
	IncludeTelegram bool
	// Logger receives one info line per applied/skipped migration. Optional.
	Logger *slog.Logger
}

func (o Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// Load reads numbered "NNNNNN_name.sql" files from dir, sorted ascending, and
// rejects duplicate version numbers. Non-SQL files, subdirectories and files
// without a numeric prefix are ignored.
func Load(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			continue
		}
		version, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || version < 1 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(data)
		result = append(result, Migration{
			Version:      version,
			Name:         entry.Name(),
			Path:         path,
			SQL:          data,
			SHA256:       hex.EncodeToString(hash[:]),
			TelegramOnly: strings.HasPrefix(entry.Name(), "000025_telegram_") || strings.HasPrefix(entry.Name(), "000026_telegram_"),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	for index := 1; index < len(result); index++ {
		if result[index-1].Version == result[index].Version {
			return nil, fmt.Errorf("duplicate migration version %d", result[index].Version)
		}
	}
	return result, nil
}

// Apply brings the database behind pool up to date with the migrations in dir.
// Already-applied migrations are verified by name + SHA-256 and a mismatch is a
// hard error. Each pending migration runs in its own transaction together with
// its bookkeeping row. The whole run holds a session advisory lock.
func Apply(ctx context.Context, pool *pgxpool.Pool, dir string, opts Options) error {
	migrations, err := Load(dir)
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, name TEXT NOT NULL, sha256_hex CHAR(64) NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = pool.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
	}()

	logger := opts.logger()
	for _, item := range migrations {
		var storedName, storedSHA string
		err := pool.QueryRow(ctx, `SELECT name, sha256_hex FROM schema_migrations WHERE version = $1`, item.Version).Scan(&storedName, &storedSHA)
		if err == nil {
			if storedSHA != item.SHA256 || storedName != item.Name {
				return fmt.Errorf("migration %d checksum or name mismatch", item.Version)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if item.TelegramOnly && !opts.IncludeTelegram {
			logger.Info("detached Telegram migration skipped", "version", item.Version, "name", item.Name)
			continue
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(item.SQL)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", item.Name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name, sha256_hex) VALUES ($1, $2, $3)`, item.Version, item.Name, item.SHA256); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		logger.Info("migration applied", "version", item.Version, "name", item.Name)
	}
	return nil
}

// Run is the command-line entry point: it loads configuration, requires
// STA_DATABASE_URL, opens a pool and calls Apply.
func Run(ctx context.Context, dir string, opts Options) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required for migrations")
	}
	startupContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := db.OpenPostgres(startupContext, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	return Apply(ctx, pool, dir, opts)
}
