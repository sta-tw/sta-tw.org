package accountapplications

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound        = errors.New("account application not found")
	ErrAlreadyDecided  = errors.New("account application already decided")
	ErrDocumentMissing = errors.New("document not found")
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("account-applications postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) Create(ctx context.Context, username, note string, emailCiphertext, emailLookupHash []byte) (Application, error) {
	var app Application
	var idText string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO account_applications (requested_username, email_ciphertext, email_lookup_hash, note)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, requested_username, note, status, created_at
	`, username, emailCiphertext, emailLookupHash, note).Scan(
		&idText, &app.RequestedUsername, &app.Note, &app.Status, &app.CreatedAt,
	)
	if err != nil {
		return Application{}, fmt.Errorf("create account application: %w", err)
	}
	app.ID, err = uuid.Parse(idText)
	if err != nil {
		return Application{}, fmt.Errorf("parse account application id: %w", err)
	}
	return app, nil
}

func (r *PostgresRepository) AddDocument(ctx context.Context, applicationID uuid.UUID, storageKey, filename, contentType string, sizeBytes int64, sha256Hex string) (Document, error) {
	var doc Document
	var idText string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO account_application_documents (application_id, object_storage_key, filename, content_type, size_bytes, sha256_hex)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, filename, content_type, size_bytes, created_at
	`, applicationID, storageKey, filename, contentType, sizeBytes, sha256Hex).Scan(
		&idText, &doc.Filename, &doc.ContentType, &doc.SizeBytes, &doc.CreatedAt,
	)
	if err != nil {
		return Document{}, fmt.Errorf("add account application document: %w", err)
	}
	doc.ID, err = uuid.Parse(idText)
	if err != nil {
		return Document{}, fmt.Errorf("parse account application document id: %w", err)
	}
	return doc, nil
}

func (r *PostgresRepository) SetTelegramMessage(ctx context.Context, applicationID uuid.UUID, chatID, messageID int64, headerText string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE account_applications SET telegram_chat_id = $2, telegram_message_id = $3, telegram_header_text = $4
		WHERE id = $1
	`, applicationID, chatID, messageID, headerText)
	if err != nil {
		return fmt.Errorf("set account application telegram message: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Get(ctx context.Context, applicationID uuid.UUID) (Application, error) {
	var app Application
	var idText string
	var createdAccountIDText *string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, requested_username, note, status, created_account_id::text,
		       telegram_chat_id, telegram_message_id, telegram_header_text, created_at
		FROM account_applications WHERE id = $1
	`, applicationID).Scan(&idText, &app.RequestedUsername, &app.Note, &app.Status, &createdAccountIDText,
		&app.TelegramChatID, &app.TelegramMessageID, &app.TelegramHeaderText, &app.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	if err != nil {
		return Application{}, fmt.Errorf("get account application: %w", err)
	}
	app.ID, err = uuid.Parse(idText)
	if err != nil {
		return Application{}, fmt.Errorf("parse account application id: %w", err)
	}
	if createdAccountIDText != nil {
		parsed, err := uuid.Parse(*createdAccountIDText)
		if err != nil {
			return Application{}, fmt.Errorf("parse account application created_account_id: %w", err)
		}
		app.CreatedAccountID = &parsed
	}
	return app, nil
}

// FindByTelegramMessage resolves the one card message an application's
// whole Telegram thread lives in (see SetTelegramMessage) back to the
// application it belongs to.
func (r *PostgresRepository) FindByTelegramMessage(ctx context.Context, chatID, messageID int64) (Application, error) {
	var app Application
	var idText string
	var createdAccountIDText *string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, requested_username, note, status, created_account_id::text,
		       telegram_chat_id, telegram_message_id, telegram_header_text, created_at
		FROM account_applications
		WHERE telegram_chat_id = $1 AND telegram_message_id = $2
	`, chatID, messageID).Scan(&idText, &app.RequestedUsername, &app.Note, &app.Status, &createdAccountIDText,
		&app.TelegramChatID, &app.TelegramMessageID, &app.TelegramHeaderText, &app.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	if err != nil {
		return Application{}, fmt.Errorf("find account application by telegram message: %w", err)
	}
	app.ID, err = uuid.Parse(idText)
	if err != nil {
		return Application{}, fmt.Errorf("parse account application id: %w", err)
	}
	if createdAccountIDText != nil {
		parsed, err := uuid.Parse(*createdAccountIDText)
		if err != nil {
			return Application{}, fmt.Errorf("parse account application created_account_id: %w", err)
		}
		app.CreatedAccountID = &parsed
	}
	return app, nil
}

func (r *PostgresRepository) ListMessages(ctx context.Context, applicationID uuid.UUID) ([]Message, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT direction, body, actor, created_at FROM account_application_messages
		WHERE application_id = $1 ORDER BY created_at
	`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list account application messages: %w", err)
	}
	defer rows.Close()
	messages := make([]Message, 0)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Direction, &m.Body, &m.Actor, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan account application message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// EmailCiphertext returns the applicant's encrypted contact email; decrypting
// it is the caller's job (crypto keys stay owned by the caller, same as how
// auth.Service handles its own email cipher).
func (r *PostgresRepository) EmailCiphertext(ctx context.Context, applicationID uuid.UUID) ([]byte, error) {
	var ciphertext []byte
	err := r.pool.QueryRow(ctx, `SELECT email_ciphertext FROM account_applications WHERE id = $1`, applicationID).Scan(&ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account application email: %w", err)
	}
	return ciphertext, nil
}

func (r *PostgresRepository) ListDocuments(ctx context.Context, applicationID uuid.UUID) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, filename, content_type, size_bytes, created_at
		FROM account_application_documents WHERE application_id = $1 ORDER BY created_at
	`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list account application documents: %w", err)
	}
	defer rows.Close()
	documents := make([]Document, 0)
	for rows.Next() {
		var doc Document
		var idText string
		if err := rows.Scan(&idText, &doc.Filename, &doc.ContentType, &doc.SizeBytes, &doc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan account application document: %w", err)
		}
		doc.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, fmt.Errorf("parse account application document id: %w", err)
		}
		documents = append(documents, doc)
	}
	return documents, rows.Err()
}

// ListDocumentStorageKeys returns the raw object-storage keys for every
// document on an application (used to build presigned download links).
func (r *PostgresRepository) ListDocumentStorageKeys(ctx context.Context, applicationID uuid.UUID) ([]DocumentKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT filename, object_storage_key FROM account_application_documents
		WHERE application_id = $1 ORDER BY created_at
	`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list account application document keys: %w", err)
	}
	defer rows.Close()
	keys := make([]DocumentKey, 0)
	for rows.Next() {
		var k DocumentKey
		if err := rows.Scan(&k.Filename, &k.StorageKey); err != nil {
			return nil, fmt.Errorf("scan account application document key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Decide atomically transitions a pending application to approved or
// rejected, failing with ErrAlreadyDecided if it isn't pending anymore (so a
// double tap on the Telegram buttons can't double-create an account).
func (r *PostgresRepository) Decide(ctx context.Context, applicationID uuid.UUID, status Status, reviewerAccountID *uuid.UUID, createdAccountID *uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE account_applications
		SET status = $2, reviewed_by = $3, reviewed_at = CURRENT_TIMESTAMP, created_account_id = $4
		WHERE id = $1 AND status = 'pending'
	`, applicationID, status, reviewerAccountID, createdAccountID)
	if err != nil {
		return fmt.Errorf("decide account application: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if _, getErr := r.Get(ctx, applicationID); getErr != nil {
			return getErr
		}
		return ErrAlreadyDecided
	}
	return nil
}

type DocumentKey struct {
	Filename   string
	StorageKey string
}

// MarkAccountIdentityVerified sets an existing active account's identity to
// 'student' — the same effect the school-email / document verification flow
// (internal/verification) has, granted here instead because the applicant
// had no usable school email to use that flow with.
func (r *PostgresRepository) MarkAccountIdentityVerified(ctx context.Context, accountID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE accounts SET identity_status = 'student', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND account_status = 'active'
	`, accountID)
	if err != nil {
		return fmt.Errorf("mark account identity verified: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) AddMessage(ctx context.Context, applicationID uuid.UUID, direction, body, sourceMessageID, actor string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO account_application_messages (application_id, direction, body, source_message_id, actor)
		VALUES ($1, $2, $3, $4, $5)
	`, applicationID, direction, body, sourceMessageID, actor)
	if err != nil {
		return fmt.Errorf("add account application message: %w", err)
	}
	return nil
}

func (r *PostgresRepository) LatestInboundMessageID(ctx context.Context, applicationID uuid.UUID) (string, error) {
	var messageID string
	err := r.pool.QueryRow(ctx, `
		SELECT source_message_id FROM account_application_messages
		WHERE application_id = $1 AND direction = 'inbound' AND source_message_id <> ''
		ORDER BY created_at DESC LIMIT 1
	`, applicationID).Scan(&messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get latest inbound account application message id: %w", err)
	}
	return messageID, nil
}
