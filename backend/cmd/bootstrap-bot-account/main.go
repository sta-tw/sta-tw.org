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

	"sta-backend/internal/auth"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
)

// bootstrap-bot-account creates a new 'service'-role account and issues it
// one API token — the escape hatch for the very first bot account, since
// creating one through the admin API needs an already-authenticated admin
// session (bootstrap-admin has the same chicken-and-egg problem for the
// very first human admin). Once at least one admin or bot account exists,
// further bot accounts can be created through the admin API instead.
//
// A 'service' account authenticates with its token alone — no password, no
// MFA (see auth.Service.RequireAdminMFA's exemption) — and by itself grants
// no endpoint access; pass -admin to also grant the 'admin' role needed to
// call admin endpoints.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	username := flag.String("username", "", "username for the new bot account")
	label := flag.String("label", "", "label for the issued API token, for operators to tell tokens apart")
	grantAdmin := flag.Bool("admin", false, "also grant the 'admin' role, so this bot can call admin endpoints")
	flag.Parse()
	if err := run(logger, *username, *label, *grantAdmin); err != nil {
		logger.Error("bot account bootstrap failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, username, label string, grantAdmin bool) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("-username is required")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "bot account"
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required for bot account bootstrap")
	}
	startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := db.OpenPostgres(startupContext, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	var fieldCipher *auth.FieldCipher
	if cfg.FieldEncryptionKeys != nil {
		fieldCipher, err = auth.NewFieldCipherRing(cfg.FieldEncryptionPrimaryVersion, cfg.FieldEncryptionKeys, cfg.EmailEncryptionKey)
	} else {
		fieldCipher, err = auth.NewFieldCipher(cfg.EmailEncryptionKey)
	}
	if err != nil {
		return err
	}
	store, err := auth.NewPostgresStore(pool)
	if err != nil {
		return err
	}
	authService, err := auth.NewService(store, fieldCipher, cfg.LookupHMACKey, cfg.SessionTTL, cfg.CookieSecure)
	if err != nil {
		return err
	}

	account, token, err := authService.CreateBotAccount(context.Background(), username, label)
	if err != nil {
		return err
	}
	if grantAdmin {
		if _, err := pool.Exec(context.Background(), `
			INSERT INTO account_roles (account_id, role)
			VALUES ($1, 'admin')
			ON CONFLICT (account_id, role) DO NOTHING
		`, account.ID); err != nil {
			return fmt.Errorf("grant admin role: %w", err)
		}
	}

	logger.Info("bot account created", "username", account.Username, "account_id", account.ID, "admin", grantAdmin)
	fmt.Println("API token (shown once, will not be recoverable afterward):")
	fmt.Println(token)
	fmt.Println()
	fmt.Println("Use it as: Authorization: Bearer " + token)
	return nil
}
