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
	"sta-backend/internal/auth"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
)

// bootstrap-ai-system grants an existing account the ai_system role and
// issues it a personal API token. Only accounts holding this role may call
// POST /api/v1/external/ai-system/brochures — the token is shown once, on
// this command's stdout, and never recoverable afterward; issue a new token
// (or revoke the old one directly in the database) if it is lost.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	username := flag.String("username", "", "existing STA username to grant the ai_system role and issue a token for")
	label := flag.String("label", "AI System", "label for the issued API token, for operators to tell tokens apart")
	flag.Parse()
	if err := run(logger, *username, *label); err != nil {
		logger.Error("ai_system bootstrap failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, username, label string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || len([]rune(username)) < 3 || len([]rune(username)) > 64 {
		return errors.New("-username must identify an existing account")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "AI System"
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required for ai_system bootstrap")
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
		VALUES ($1, 'ai_system')
		ON CONFLICT (account_id, role) DO NOTHING
	`, accountID); err != nil {
		return err
	}

	token, err := auth.NewOpaqueToken(32)
	if err != nil {
		return fmt.Errorf("generate api token: %w", err)
	}
	hash := auth.HashOpaqueToken(token)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO account_api_tokens (account_id, label, token_hash)
		VALUES ($1, $2, $3)
	`, accountID, label, hash); err != nil {
		return fmt.Errorf("store api token: %w", err)
	}

	logger.Info("ai_system role ensured and API token issued", "username", username, "label", label)
	fmt.Println("API token (shown once, will not be recoverable afterward):")
	fmt.Println(token)
	return nil
}
