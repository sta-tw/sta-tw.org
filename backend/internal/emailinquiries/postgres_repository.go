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

func (r *PostgresRepository) Create(ctx context.Context, mailRouteID uuid.UUID, note string, emailCiphertext, emailLookupHash []byte) (Inquiry, error) {
	var inquiry Inquiry
	var idText string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO email_inquiries (mail_route_id, email_ciphertext, email_lookup_hash, note)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, mail_route_id, note, created_at
	`, mailRouteID, emailCiphertext, emailLookupHash, note).Scan(&idText, &inquiry.MailRouteID, &inquiry.Note, &inquiry.CreatedAt)
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

func (r *PostgresRepository) SetDiscordThread(ctx context.Context, inquiryID uuid.UUID, threadID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE email_inquiries SET discord_thread_id = $2 WHERE id = $1
	`, inquiryID, threadID)
	if err != nil {
		return fmt.Errorf("set email inquiry discord thread: %w", err)
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
		SELECT id::text, mail_route_id, note, discord_thread_id, created_at
		FROM email_inquiries WHERE id = $1
	`, inquiryID).Scan(&idText, &inquiry.MailRouteID, &inquiry.Note, &inquiry.DiscordThreadID, &inquiry.CreatedAt)
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

// ReplyTemplate resolves the template a /re reply for mailRouteID should
// use: the route's own mail_routes.reply_template override if it has one
// (internal/mailroutes' create/update, non-empty), else the global
// app_settings row (see internal/admin/settings.go's
// getMailReplyTemplate/setMailReplyTemplate, the only writer of that key).
func (r *PostgresRepository) ReplyTemplate(ctx context.Context, mailRouteID uuid.UUID) (string, bool, error) {
	var value *string
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT reply_template FROM mail_routes WHERE id = $1 AND reply_template IS NOT NULL AND reply_template <> ''),
			(SELECT value FROM app_settings WHERE key = 'mail_reply_template')
		)
	`, mailRouteID).Scan(&value)
	if err != nil {
		return "", false, fmt.Errorf("get mail reply template: %w", err)
	}
	if value == nil {
		return "", false, nil
	}
	return *value, true, nil
}

// ListByMailRoute returns every inquiry filed under mailRouteID, newest
// first — the admin UI's "mail history" view for a mail_routes row.
func (r *PostgresRepository) ListByMailRoute(ctx context.Context, mailRouteID uuid.UUID) ([]Inquiry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, mail_route_id, note, discord_thread_id, created_at
		FROM email_inquiries
		WHERE mail_route_id = $1
		ORDER BY created_at DESC
	`, mailRouteID)
	if err != nil {
		return nil, fmt.Errorf("list email inquiries by mail route: %w", err)
	}
	defer rows.Close()
	items := make([]Inquiry, 0)
	for rows.Next() {
		var inquiry Inquiry
		var idText string
		if err := rows.Scan(&idText, &inquiry.MailRouteID, &inquiry.Note, &inquiry.DiscordThreadID, &inquiry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan email inquiry: %w", err)
		}
		if inquiry.ID, err = uuid.Parse(idText); err != nil {
			return nil, fmt.Errorf("parse email inquiry id: %w", err)
		}
		items = append(items, inquiry)
	}
	return items, rows.Err()
}

// FindByDiscordThread resolves the Discord forum thread an inquiry's whole
// conversation lives in (see SetDiscordThread) back to the inquiry it
// belongs to — used by the /re slash command handler.
func (r *PostgresRepository) FindByDiscordThread(ctx context.Context, threadID string) (Inquiry, error) {
	var inquiry Inquiry
	var idText string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, mail_route_id, note, discord_thread_id, created_at
		FROM email_inquiries
		WHERE discord_thread_id = $1
	`, threadID).Scan(&idText, &inquiry.MailRouteID, &inquiry.Note, &inquiry.DiscordThreadID, &inquiry.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Inquiry{}, ErrNotFound
	}
	if err != nil {
		return Inquiry{}, fmt.Errorf("find email inquiry by discord thread: %w", err)
	}
	inquiry.ID, err = uuid.Parse(idText)
	if err != nil {
		return Inquiry{}, fmt.Errorf("parse email inquiry id: %w", err)
	}
	return inquiry, nil
}

func (r *PostgresRepository) ListMessages(ctx context.Context, inquiryID uuid.UUID) ([]Message, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT direction, subject, body, actor, actor_discord_id, to_address, sender_ip, auth_results,
		       source_message_id, in_reply_to, email_references, mailer, created_at
		FROM email_inquiry_messages
		WHERE inquiry_id = $1 ORDER BY created_at
	`, inquiryID)
	if err != nil {
		return nil, fmt.Errorf("list email inquiry messages: %w", err)
	}
	defer rows.Close()
	messages := make([]Message, 0)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Direction, &m.Subject, &m.Body, &m.Actor, &m.ActorDiscordID, &m.ToAddress, &m.SenderIP, &m.AuthResults,
			&m.SourceMessageID, &m.InReplyTo, &m.References, &m.Mailer, &m.CreatedAt); err != nil {
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

func (r *PostgresRepository) AddMessage(ctx context.Context, inquiryID uuid.UUID, direction, body, sourceMessageID, actor string, meta MessageMeta) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO email_inquiry_messages (
			inquiry_id, direction, body, source_message_id, actor, subject, sender_ip, auth_results,
			actor_discord_id, to_address, in_reply_to, email_references, mailer
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, inquiryID, direction, body, sourceMessageID, actor, meta.Subject, meta.SenderIP, meta.AuthResults,
		meta.ActorDiscordID, meta.ToAddress, meta.InReplyTo, meta.References, meta.Mailer)
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
