package emailinquiries

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("email inquiry not found")

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("email-inquiries postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) Create(ctx context.Context, note string, emailCiphertext, emailLookupHash []byte) (Inquiry, error) {
	var inquiry Inquiry
	var idText string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO email_inquiries (email_ciphertext, email_lookup_hash, note)
		VALUES ($1, $2, $3)
		RETURNING id::text, note, created_at
	`, emailCiphertext, emailLookupHash, note).Scan(&idText, &inquiry.Note, &inquiry.CreatedAt)
	if err != nil {
		return Inquiry{}, fmt.Errorf("create email inquiry: %w", err)
	}
	inquiry.ID, err = uuid.Parse(idText)
	if err != nil {
		return Inquiry{}, fmt.Errorf("parse email inquiry id: %w", err)
	}
	return inquiry, nil
}

func (r *PostgresRepository) AddDocument(ctx context.Context, inquiryID uuid.UUID, storageKey, filename, contentType string, sizeBytes int64, sha256Hex string) (Document, error) {
	var doc Document
	var idText string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO email_inquiry_documents (inquiry_id, object_storage_key, filename, content_type, size_bytes, sha256_hex)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, filename, content_type, size_bytes, created_at
	`, inquiryID, storageKey, filename, contentType, sizeBytes, sha256Hex).Scan(
		&idText, &doc.Filename, &doc.ContentType, &doc.SizeBytes, &doc.CreatedAt,
	)
	if err != nil {
		return Document{}, fmt.Errorf("add email inquiry document: %w", err)
	}
	doc.ID, err = uuid.Parse(idText)
	if err != nil {
		return Document{}, fmt.Errorf("parse email inquiry document id: %w", err)
	}
	return doc, nil
}

func (r *PostgresRepository) SetTelegramMessage(ctx context.Context, inquiryID uuid.UUID, chatID, messageID int64, headerText string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE email_inquiries SET telegram_chat_id = $2, telegram_message_id = $3, telegram_header_text = $4
		WHERE id = $1
	`, inquiryID, chatID, messageID, headerText)
	if err != nil {
		return fmt.Errorf("set email inquiry telegram message: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Get(ctx context.Context, inquiryID uuid.UUID) (Inquiry, error) {
	var inquiry Inquiry
	var idText string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, note, telegram_chat_id, telegram_message_id, telegram_header_text, created_at
		FROM email_inquiries WHERE id = $1
	`, inquiryID).Scan(&idText, &inquiry.Note, &inquiry.TelegramChatID, &inquiry.TelegramMessageID, &inquiry.TelegramHeaderText, &inquiry.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Inquiry{}, ErrNotFound
	}
	if err != nil {
		return Inquiry{}, fmt.Errorf("get email inquiry: %w", err)
	}
	inquiry.ID, err = uuid.Parse(idText)
	if err != nil {
		return Inquiry{}, fmt.Errorf("parse email inquiry id: %w", err)
	}
	return inquiry, nil
}

// FindByTelegramMessage resolves the one card message an inquiry's whole
// Telegram thread lives in (see SetTelegramMessage) back to the inquiry it
// belongs to.
func (r *PostgresRepository) FindByTelegramMessage(ctx context.Context, chatID, messageID int64) (Inquiry, error) {
	var inquiry Inquiry
	var idText string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, note, telegram_chat_id, telegram_message_id, telegram_header_text, created_at
		FROM email_inquiries
		WHERE telegram_chat_id = $1 AND telegram_message_id = $2
	`, chatID, messageID).Scan(&idText, &inquiry.Note, &inquiry.TelegramChatID, &inquiry.TelegramMessageID, &inquiry.TelegramHeaderText, &inquiry.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Inquiry{}, ErrNotFound
	}
	if err != nil {
		return Inquiry{}, fmt.Errorf("find email inquiry by telegram message: %w", err)
	}
	inquiry.ID, err = uuid.Parse(idText)
	if err != nil {
		return Inquiry{}, fmt.Errorf("parse email inquiry id: %w", err)
	}
	return inquiry, nil
}

func (r *PostgresRepository) ListMessages(ctx context.Context, inquiryID uuid.UUID) ([]Message, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT direction, body, actor, created_at FROM email_inquiry_messages
		WHERE inquiry_id = $1 ORDER BY created_at
	`, inquiryID)
	if err != nil {
		return nil, fmt.Errorf("list email inquiry messages: %w", err)
	}
	defer rows.Close()
	messages := make([]Message, 0)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Direction, &m.Body, &m.Actor, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan email inquiry message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// EmailCiphertext returns the sender's encrypted contact email; decrypting
// it is the caller's job (crypto keys stay owned by the caller).
func (r *PostgresRepository) EmailCiphertext(ctx context.Context, inquiryID uuid.UUID) ([]byte, error) {
	var ciphertext []byte
	err := r.pool.QueryRow(ctx, `SELECT email_ciphertext FROM email_inquiries WHERE id = $1`, inquiryID).Scan(&ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get email inquiry email: %w", err)
	}
	return ciphertext, nil
}

func (r *PostgresRepository) AddMessage(ctx context.Context, inquiryID uuid.UUID, direction, body, sourceMessageID, actor string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO email_inquiry_messages (inquiry_id, direction, body, source_message_id, actor)
		VALUES ($1, $2, $3, $4, $5)
	`, inquiryID, direction, body, sourceMessageID, actor)
	if err != nil {
		return fmt.Errorf("add email inquiry message: %w", err)
	}
	return nil
}

func (r *PostgresRepository) LatestInboundMessageID(ctx context.Context, inquiryID uuid.UUID) (string, error) {
	var messageID string
	err := r.pool.QueryRow(ctx, `
		SELECT source_message_id FROM email_inquiry_messages
		WHERE inquiry_id = $1 AND direction = 'inbound' AND source_message_id <> ''
		ORDER BY created_at DESC LIMIT 1
	`, inquiryID).Scan(&messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get latest inbound email inquiry message id: %w", err)
	}
	return messageID, nil
}
