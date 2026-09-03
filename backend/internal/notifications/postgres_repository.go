package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/auth"
)

// eventPublisher is satisfied by *events.Hub. It lets a freshly created in-app
// notification wake a live SSE stream without this package importing the hub.
type eventPublisher interface {
	PublishData(ctx context.Context, topic, kind string, data any) error
}

type PostgresRepository struct {
	pool      *pgxpool.Pool
	cipher    *auth.FieldCipher
	publisher eventPublisher
}

func NewPostgresRepository(pool *pgxpool.Pool, cipher *auth.FieldCipher) (*PostgresRepository, error) {
	if pool == nil || cipher == nil {
		return nil, errors.New("notification repository dependencies are missing")
	}
	return &PostgresRepository{pool: pool, cipher: cipher}, nil
}

// SetEventPublisher wires an SSE hub so CreateInApp emits a live event.
func (r *PostgresRepository) SetEventPublisher(p eventPublisher) { r.publisher = p }

func (r *PostgresRepository) CreateInApp(ctx context.Context, accountID uuid.UUID, kind, dedupKey, title, body string) (Notification, error) {
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(dedupKey) == "" || strings.TrimSpace(title) == "" || strings.TrimSpace(body) == "" {
		return Notification{}, errors.New("notification data is invalid")
	}
	titleCiphertext, err := r.cipher.Seal(title)
	if err != nil {
		return Notification{}, err
	}
	bodyCiphertext, err := r.cipher.Seal(body)
	if err != nil {
		return Notification{}, err
	}
	var notification Notification
	var idText string
	err = r.pool.QueryRow(ctx, `
		INSERT INTO notifications (account_id, kind, dedup_key, title_ciphertext, body_ciphertext)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (account_id, dedup_key) DO UPDATE SET dedup_key = EXCLUDED.dedup_key
		RETURNING id::text, kind, title_ciphertext, body_ciphertext, read_at, created_at
	`, accountID, kind, dedupKey, titleCiphertext, bodyCiphertext).Scan(&idText, &notification.Kind, &titleCiphertext, &bodyCiphertext, &notification.ReadAt, &notification.CreatedAt)
	if err != nil {
		return Notification{}, mapNotificationError(err)
	}
	notification.ID, err = uuid.Parse(idText)
	if err != nil {
		return Notification{}, err
	}
	notification.Title, err = r.cipher.Open(titleCiphertext)
	if err != nil {
		return Notification{}, err
	}
	notification.Body, err = r.cipher.Open(bodyCiphertext)
	if err != nil {
		return Notification{}, err
	}
	if r.publisher != nil {
		// Best effort: never fail the write because the live channel is down.
		_ = r.publisher.PublishData(ctx, "notifications:"+accountID.String(), "notification.created", map[string]any{
			"id":         notification.ID,
			"kind":       notification.Kind,
			"created_at": notification.CreatedAt,
		})
	}
	return notification, nil
}

func (r *PostgresRepository) EnqueueEmailForAccount(ctx context.Context, accountID uuid.UUID, dedupKey, subject, body, kind string) error {
	var recipientCiphertext []byte
	if err := r.pool.QueryRow(ctx, `SELECT email_ciphertext FROM accounts WHERE id = $1 AND account_status = 'active'`, accountID).Scan(&recipientCiphertext); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return r.enqueueEmail(ctx, accountID, nil, recipientCiphertext, dedupKey, subject, body, kind)
}

func (r *PostgresRepository) EnqueueEmailTo(ctx context.Context, accountID uuid.UUID, recipientCiphertext []byte, dedupKey, subject, body string) error {
	return r.enqueueEmail(ctx, accountID, nil, recipientCiphertext, dedupKey, subject, body, "verification")
}

func (r *PostgresRepository) enqueueEmail(ctx context.Context, accountID uuid.UUID, notificationID *uuid.UUID, recipientCiphertext []byte, dedupKey, subject, body, kind string) error {
	if len(recipientCiphertext) == 0 || strings.TrimSpace(dedupKey) == "" || strings.TrimSpace(subject) == "" || strings.TrimSpace(body) == "" || strings.TrimSpace(kind) == "" {
		return errors.New("email notification data is invalid")
	}
	payload, err := json.Marshal(EmailPayload{Subject: subject, Text: body})
	if err != nil {
		return err
	}
	payloadCiphertext, err := r.cipher.Seal(string(payload))
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO email_outbox (account_id, notification_id, dedup_key, recipient_ciphertext, payload_ciphertext)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (account_id, dedup_key) DO NOTHING
	`, accountID, notificationID, dedupKey, recipientCiphertext, payloadCiphertext)
	return mapNotificationError(err)
}

func (r *PostgresRepository) List(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]Notification, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, kind, title_ciphertext, body_ciphertext, read_at, created_at
		FROM notifications WHERE account_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, accountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Notification, 0)
	for rows.Next() {
		var item Notification
		var idText string
		var titleCiphertext, bodyCiphertext []byte
		if err := rows.Scan(&idText, &item.Kind, &titleCiphertext, &bodyCiphertext, &item.ReadAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, err
		}
		item.Title, err = r.cipher.Open(titleCiphertext)
		if err != nil {
			return nil, err
		}
		item.Body, err = r.cipher.Open(bodyCiphertext)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) MarkRead(ctx context.Context, accountID, notificationID uuid.UUID) error {
	command, err := r.pool.Exec(ctx, `UPDATE notifications SET read_at = COALESCE(read_at, CURRENT_TIMESTAMP) WHERE id = $1 AND account_id = $2`, notificationID, accountID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ClaimEmailOutbox(ctx context.Context, limit int) ([]EmailTask, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT id::text, recipient_ciphertext, payload_ciphertext
		FROM email_outbox
		WHERE (status IN ('pending', 'failed') AND available_at <= CURRENT_TIMESTAMP)
		   OR (status = 'processing' AND updated_at < CURRENT_TIMESTAMP - INTERVAL '5 minutes')
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, err
	}
	tasks := make([]EmailTask, 0, limit)
	for rows.Next() {
		var task EmailTask
		var idText string
		if err := rows.Scan(&idText, &task.RecipientCiphertext, &task.PayloadCiphertext); err != nil {
			rows.Close()
			return nil, err
		}
		task.ID, err = uuid.Parse(idText)
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
		if _, err := tx.Exec(ctx, `UPDATE email_outbox SET status = 'processing', attempt_count = attempt_count + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, task.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *PostgresRepository) MarkEmailSent(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE email_outbox SET status = 'sent', last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id)
	return err
}

func (r *PostgresRepository) MarkEmailFailed(ctx context.Context, id uuid.UUID, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE email_outbox
		SET status = CASE WHEN attempt_count >= max_attempts THEN 'abandoned' ELSE 'failed' END,
		    last_error = $2,
		    available_at = CURRENT_TIMESTAMP + (INTERVAL '30 seconds' * LEAST(GREATEST(attempt_count, 1), 10)),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, id, message)
	return err
}

func (r *PostgresRepository) ClaimInquiryNotifications(ctx context.Context, limit int) ([]InquiryNotificationTask, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT i.id::text, a.account_id::text, i.application_id::text, i.inquiry_round, i.response_deadline
		FROM willingness_inquiries i
		JOIN applications a ON a.id = i.application_id
		WHERE ((i.notification_status IN ('pending', 'failed') AND i.notification_available_at <= CURRENT_TIMESTAMP)
		   OR (i.notification_status = 'processing' AND i.updated_at < CURRENT_TIMESTAMP - INTERVAL '5 minutes'))
		  AND NOT EXISTS (SELECT 1 FROM willingness_response_events e WHERE e.inquiry_id = i.id)
		ORDER BY i.created_at
		LIMIT $1
		FOR UPDATE OF i SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, err
	}
	tasks := make([]InquiryNotificationTask, 0, limit)
	for rows.Next() {
		var task InquiryNotificationTask
		var idText, accountIDText, applicationIDText string
		if err := rows.Scan(&idText, &accountIDText, &applicationIDText, &task.InquiryRound, &task.ResponseDeadline); err != nil {
			rows.Close()
			return nil, err
		}
		if task.ID, err = uuid.Parse(idText); err != nil {
			rows.Close()
			return nil, err
		}
		if task.AccountID, err = uuid.Parse(accountIDText); err != nil {
			rows.Close()
			return nil, err
		}
		if task.ApplicationID, err = uuid.Parse(applicationIDText); err != nil {
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
		if _, err := tx.Exec(ctx, `UPDATE willingness_inquiries SET notification_status = 'processing', notification_attempt_count = notification_attempt_count + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, task.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *PostgresRepository) MarkInquiryNotificationEnqueued(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE willingness_inquiries SET notification_status = 'enqueued', notification_enqueued_at = CURRENT_TIMESTAMP, notification_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id)
	return err
}

func (r *PostgresRepository) MarkInquiryNotificationFailed(ctx context.Context, id uuid.UUID, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE willingness_inquiries
		SET notification_status = CASE WHEN notification_attempt_count >= notification_max_attempts THEN 'abandoned' ELSE 'failed' END,
		    notification_error = $2,
		    notification_available_at = CURRENT_TIMESTAMP + (INTERVAL '30 seconds' * LEAST(GREATEST(notification_attempt_count, 1), 10)),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, id, message)
	return err
}

func mapNotificationError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return fmt.Errorf("notification database operation: %w", err)
}
