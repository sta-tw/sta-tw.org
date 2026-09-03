package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/auth"
	"sta-backend/internal/pagination"
)

type eventPublisher interface {
	PublishData(ctx context.Context, topic, kind string, data any) error
}

type PostgresRepository struct {
	pool      *pgxpool.Pool
	lookupKey []byte
	publisher eventPublisher
}

func NewPostgresRepository(pool *pgxpool.Pool, lookupKey []byte) (*PostgresRepository, error) {
	if pool == nil || len(lookupKey) != 32 {
		return nil, errors.New("chat repository dependencies are missing")
	}
	return &PostgresRepository{pool: pool, lookupKey: append([]byte(nil), lookupKey...)}, nil
}

// SetEventPublisher wires an SSE hub so new lounge messages emit a live event.
func (r *PostgresRepository) SetEventPublisher(p eventPublisher) { r.publisher = p }

func (r *PostgresRepository) announce(ctx context.Context, message Message) {
	if r.publisher == nil {
		return
	}
	_ = r.publisher.PublishData(ctx, "chat:lounge", "chat.message", map[string]any{
		"id":              message.ID,
		"body":            message.Body,
		"source_platform": message.SourcePlatform,
		"status":          message.Status,
		"created_at":      message.CreatedAt,
		"edited_at":       message.EditedAt,
	})
}

func (r *PostgresRepository) CreateWebsiteMessage(ctx context.Context, accountID uuid.UUID, body string) (Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback(ctx)
	channelID, err := r.ensureChannel(ctx, tx)
	if err != nil {
		return Message{}, err
	}
	message, err := insertMessage(ctx, tx, channelID, &accountID, PlatformWebsite, nil, body)
	if err != nil {
		return Message{}, err
	}
	if err := createOutboundTasks(ctx, tx, message.ID, OperationCreate, message.Body); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	r.announce(ctx, message)
	return message, nil
}

func (r *PostgresRepository) ApplyExternalMessage(ctx context.Context, input ExternalMessage, authorHashKey []byte) (Message, error) {
	if err := ValidateExternalMessage(input); err != nil {
		return Message{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback(ctx)
	channelID, err := r.ensureChannel(ctx, tx)
	if err != nil {
		return Message{}, err
	}
	var existingIDText string
	err = tx.QueryRow(ctx, `SELECT message_id::text FROM chat_inbound_dedup WHERE platform = $1 AND external_message_id = $2 FOR UPDATE`, input.Platform, input.ExternalMessageID).Scan(&existingIDText)
	if errors.Is(err, pgx.ErrNoRows) {
		externalHash, hashErr := auth.LookupHash(authorHashKey, input.ExternalAuthorID)
		if hashErr != nil {
			return Message{}, hashErr
		}
		message, insertErr := insertMessage(ctx, tx, channelID, nil, input.Platform, externalHash, input.Body)
		if insertErr != nil {
			return Message{}, insertErr
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chat_inbound_dedup (platform, external_message_id, message_id) VALUES ($1, $2, $3)`, input.Platform, input.ExternalMessageID, message.ID); err != nil {
			return Message{}, mapChatError(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chat_message_bridges (message_id, platform, external_message_id, sync_status, synced_at) VALUES ($1, $2, $3, 'sent', CURRENT_TIMESTAMP)`, message.ID, input.Platform, input.ExternalMessageID); err != nil {
			return Message{}, mapChatError(err)
		}
		if err := createOutboundTasksExcept(ctx, tx, message.ID, input.Platform, OperationCreate, message.Body); err != nil {
			return Message{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Message{}, err
		}
		r.announce(ctx, message)
		return message, nil
	} else if err != nil {
		return Message{}, err
	}
	existingID, err := uuid.Parse(existingIDText)
	if err != nil {
		return Message{}, err
	}
	if input.Operation == OperationCreate {
		message, err := getMessage(ctx, tx, existingID)
		if err != nil {
			return Message{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Message{}, err
		}
		return message, nil
	}
	message, err := updateExistingMessage(ctx, tx, existingID, input.Operation, input.Body)
	if err != nil {
		return Message{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE chat_message_bridges SET sync_status = $3, synced_at = CURRENT_TIMESTAMP, last_error = NULL WHERE message_id = $1 AND platform = $2`, message.ID, input.Platform, bridgeStatus(input.Operation)); err != nil {
		return Message{}, err
	}
	if err := createOutboundTasksExcept(ctx, tx, message.ID, input.Platform, input.Operation, input.Body); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	r.announce(ctx, message)
	return message, nil
}

func getMessage(ctx context.Context, tx dbTx, messageID uuid.UUID) (Message, error) {
	var message Message
	var idText string
	err := tx.QueryRow(ctx, `SELECT id::text, body, source_platform, status, created_at, edited_at FROM chat_messages WHERE id = $1`, messageID).Scan(&idText, &message.Body, &message.SourcePlatform, &message.Status, &message.CreatedAt, &message.EditedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, err
	}
	message.ID, err = uuid.Parse(idText)
	return message, err
}

func (r *PostgresRepository) ListMessages(ctx context.Context, limit int, after pagination.Cursor) ([]Message, string, error) {
	limit = pagination.ClampLimit(limit, 50, 100)
	var afterTime *time.Time
	var afterID *uuid.UUID
	if !after.Zero() {
		t := after.Time
		id := after.UUID()
		afterTime, afterID = &t, &id
	}
	rows, err := r.pool.Query(ctx, `
		SELECT m.id::text, m.body, m.source_platform, m.status, m.created_at, m.edited_at
		FROM chat_messages m
		JOIN chat_channels c ON c.id = m.channel_id AND c.channel_key = 'lounge'
		WHERE m.status <> 'deleted'
		  AND ($2::timestamptz IS NULL OR (m.created_at, m.id) < ($2::timestamptz, $3::uuid))
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $1
	`, limit, afterTime, afterID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	result := make([]Message, 0)
	for rows.Next() {
		var message Message
		var idText string
		if err := rows.Scan(&idText, &message.Body, &message.SourcePlatform, &message.Status, &message.CreatedAt, &message.EditedAt); err != nil {
			return nil, "", err
		}
		message.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, "", err
		}
		result = append(result, message)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var next string
	if n := len(result); n > 0 {
		last := result[n-1]
		next = pagination.Next(n, limit, last.CreatedAt, last.ID)
	}
	return result, next, nil
}

func (r *PostgresRepository) ClaimOutbox(ctx context.Context, limit int) ([]OutboxTask, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT o.id::text, o.message_id::text, o.target_platform, o.operation,
		       COALESCE(o.payload->>'body', ''), COALESCE(b.external_message_id, '')
		FROM chat_sync_outbox o
		LEFT JOIN chat_message_bridges b ON b.message_id = o.message_id AND b.platform = o.target_platform
		WHERE (
			(o.status IN ('pending', 'failed') AND o.available_at <= CURRENT_TIMESTAMP)
			OR (o.status = 'processing' AND o.updated_at < CURRENT_TIMESTAMP - INTERVAL '5 minutes')
		)
		  AND NOT EXISTS (
			SELECT 1
			FROM chat_sync_outbox previous
			WHERE previous.message_id = o.message_id
			  AND previous.target_platform = o.target_platform
			  AND previous.id <> o.id
			  AND previous.created_at < o.created_at
			  AND previous.status <> 'sent'
		  )
		ORDER BY o.created_at
		LIMIT $1
		FOR UPDATE OF o SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, err
	}
	tasks := make([]OutboxTask, 0, limit)
	for rows.Next() {
		var task OutboxTask
		var idText, messageIDText string
		if err := rows.Scan(&idText, &messageIDText, &task.TargetPlatform, &task.Operation, &task.Body, &task.ExternalMessageID); err != nil {
			rows.Close()
			return nil, err
		}
		task.ID, err = uuid.Parse(idText)
		if err != nil {
			rows.Close()
			return nil, err
		}
		task.MessageID, err = uuid.Parse(messageIDText)
		if err != nil {
			rows.Close()
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, task := range tasks {
		if _, err := tx.Exec(ctx, `UPDATE chat_sync_outbox SET status = 'processing', attempt_count = attempt_count + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, task.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *PostgresRepository) MarkOutboxSent(ctx context.Context, task OutboxTask, externalID string) error {
	if strings.TrimSpace(externalID) == "" {
		return errors.New("external message id is required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO chat_message_bridges (message_id, platform, external_message_id, sync_status, synced_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (message_id, platform) DO UPDATE SET external_message_id = EXCLUDED.external_message_id, sync_status = EXCLUDED.sync_status, synced_at = CURRENT_TIMESTAMP, last_error = NULL
	`, task.MessageID, task.TargetPlatform, externalID, bridgeStatus(task.Operation)); err != nil {
		return mapChatError(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE chat_sync_outbox SET status = 'sent', updated_at = CURRENT_TIMESTAMP WHERE id = $1`, task.ID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (r *PostgresRepository) MarkOutboxFailed(ctx context.Context, task OutboxTask, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE chat_sync_outbox
		SET status = CASE WHEN attempt_count >= max_attempts THEN 'abandoned' ELSE 'failed' END,
		    last_error = $2,
		    available_at = CURRENT_TIMESTAMP + (INTERVAL '30 seconds' * LEAST(GREATEST(attempt_count, 1), 10)),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, task.ID, message)
	return err
}

func bridgeStatus(operation Operation) string {
	switch operation {
	case OperationEdit:
		return "edited"
	case OperationDelete:
		return "deleted"
	default:
		return "sent"
	}
}

type dbTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *PostgresRepository) ensureChannel(ctx context.Context, tx dbTx) (uuid.UUID, error) {
	var idText string
	err := tx.QueryRow(ctx, `
		INSERT INTO chat_channels (channel_key, display_name)
		VALUES ('lounge', '閒聊')
		ON CONFLICT (channel_key) DO UPDATE SET is_active = TRUE
		RETURNING id::text
	`).Scan(&idText)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(idText)
}

func insertMessage(ctx context.Context, tx dbTx, channelID uuid.UUID, accountID *uuid.UUID, platform Platform, externalAuthorHash []byte, body string) (Message, error) {
	var message Message
	var idText string
	err := tx.QueryRow(ctx, `
		INSERT INTO chat_messages (channel_id, author_account_id, source_platform, external_author_hash, body)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, body, source_platform, status, created_at, edited_at
	`, channelID, accountID, platform, externalAuthorHash, body).Scan(&idText, &message.Body, &message.SourcePlatform, &message.Status, &message.CreatedAt, &message.EditedAt)
	if err != nil {
		return Message{}, mapChatError(err)
	}
	message.ID, err = uuid.Parse(idText)
	return message, err
}

func updateExistingMessage(ctx context.Context, tx dbTx, messageID uuid.UUID, operation Operation, body string) (Message, error) {
	var message Message
	var idText string
	if operation == OperationDelete {
		err := tx.QueryRow(ctx, `UPDATE chat_messages SET status = 'deleted', body = '', deleted_at = CURRENT_TIMESTAMP WHERE id = $1 RETURNING id::text, body, source_platform, status, created_at, edited_at`, messageID).Scan(&idText, &message.Body, &message.SourcePlatform, &message.Status, &message.CreatedAt, &message.EditedAt)
		if err != nil {
			return Message{}, mapChatError(err)
		}
	} else {
		err := tx.QueryRow(ctx, `UPDATE chat_messages SET body = $2, status = 'edited', edited_at = CURRENT_TIMESTAMP WHERE id = $1 RETURNING id::text, body, source_platform, status, created_at, edited_at`, messageID, body).Scan(&idText, &message.Body, &message.SourcePlatform, &message.Status, &message.CreatedAt, &message.EditedAt)
		if err != nil {
			return Message{}, mapChatError(err)
		}
	}
	parsedID, err := uuid.Parse(idText)
	if err != nil {
		return Message{}, fmt.Errorf("parse chat message id: %w", err)
	}
	message.ID = parsedID
	return message, nil
}

func createOutboundTasks(ctx context.Context, tx dbTx, messageID uuid.UUID, operation Operation, body string) error {
	return createOutboundTasksExcept(ctx, tx, messageID, "", operation, body)
}

func createOutboundTasksExcept(ctx context.Context, tx dbTx, messageID uuid.UUID, except Platform, operation Operation, body string) error {
	for _, platform := range []Platform{PlatformDiscord, PlatformTelegram} {
		if platform == except {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO chat_sync_outbox (message_id, target_platform, operation, payload)
			VALUES ($1, $2, $3, jsonb_build_object('message_id', $1::uuid, 'body', $4::text, 'operation', $3::varchar))
		`, messageID, platform, operation, body); err != nil {
			return fmt.Errorf("create chat sync outbox: %w", err)
		}
	}
	return nil
}

func mapChatError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrNotFound
		}
	}
	return err
}
