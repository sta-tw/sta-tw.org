package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	username := flag.String("username", "", "existing STA username to grant the admin role")
	flag.Parse()
	if err := run(logger, *username); err != nil {
		logger.Error("admin bootstrap failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, username string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || len([]rune(username)) < 3 || len([]rune(username)) > 64 {
		return errors.New("-username must identify an existing account")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required for admin bootstrap")
	}
	startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := db.OpenPostgres(startupContext, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	var accountID string
	err = pool.QueryRow(context.Background(), `SELECT id::text FROM accounts WHERE username = $1 AND account_status = 'active'`, username).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("active account %q was not found", username)
	}
	if err != nil {
		return err
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO account_roles (account_id, role)
		VALUES ($1, 'admin')
		ON CONFLICT (account_id, role) DO NOTHING
	`, accountID); err != nil {
		return err
	}
	logger.Info("admin role ensured", "username", username)
	return nil
}
