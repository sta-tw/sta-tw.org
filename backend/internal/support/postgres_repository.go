package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/auth"
)

type PostgresRepository struct {
	pool          *pgxpool.Pool
	cipher        *auth.FieldCipher
	lookupKey     []byte
	supportEmail  string
	publicBaseURL string
}

func NewPostgresRepository(pool *pgxpool.Pool, cipher *auth.FieldCipher, lookupKey []byte, supportEmail, publicBaseURL string) (*PostgresRepository, error) {
	if pool == nil || cipher == nil {
		return nil, errors.New("support repository dependencies are missing")
	}
	return &PostgresRepository{
		pool:          pool,
		cipher:        cipher,
		lookupKey:     append([]byte(nil), lookupKey...),
		supportEmail:  strings.TrimSpace(supportEmail),
		publicBaseURL: strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
	}, nil
}

func (r *PostgresRepository) CreateTicket(ctx context.Context, accountID uuid.UUID, input CreateTicketInput) (TicketDetail, error) {
	return r.CreateTicketWithAttachments(ctx, accountID, input, nil)
}

func (r *PostgresRepository) CreateTicketWithAttachments(ctx context.Context, accountID uuid.UUID, input CreateTicketInput, attachments []AttachmentInput) (TicketDetail, error) {
	if err := ValidateCreateTicket(input); err != nil {
		return TicketDetail{}, err
	}
	if err := ValidateAttachments(attachments); err != nil {
		return TicketDetail{}, err
	}
	input.Category = strings.TrimSpace(input.Category)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TicketDetail{}, fmt.Errorf("begin support ticket: %w", err)
	}
	defer tx.Rollback(ctx)

	ticket, err := insertTicket(ctx, tx, accountID, input)
	if err != nil {
		return TicketDetail{}, mapSupportError(err)
	}
	message, err := insertSupportMessage(ctx, tx, ticket.ID, &accountID, "user", string(SourceWebsite), nil, input.Body, time.Now().UTC())
	if err != nil {
		return TicketDetail{}, fmt.Errorf("create support ticket message: %w", err)
	}
	message.Attachments, err = insertSupportAttachments(ctx, tx, ticket.ID, message.ID, attachments)
	if err != nil {
		return TicketDetail{}, err
	}
	if err := addTicketEvent(ctx, tx, ticket.ID, &accountID, "created", map[string]any{
		"category": input.Category,
		"subject":  input.Subject,
	}); err != nil {
		return TicketDetail{}, err
	}
	if err := enqueueDiscordChannelTx(ctx, tx, ticket.ID, ticket.TicketNumber, ticket.Subject); err != nil {
		return TicketDetail{}, err
	}
	if err := r.enqueueOfficialEmailTx(ctx, tx, ticket, "new", "客服收到新的 Ticket "+ticket.TicketNumber, formatOfficialBody(ticket, input.Body)); err != nil {
		return TicketDetail{}, err
	}
	if err := r.enqueueUserNoticeTx(ctx, tx, ticket, "created", "客服單已建立", "我們已收到你的客服問題，Ticket 編號是 "+ticket.TicketNumber+"。", &accountID); err != nil {
		return TicketDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TicketDetail{}, fmt.Errorf("commit support ticket: %w", err)
	}
	return TicketDetail{Ticket: ticket, Messages: []Message{message}}, nil
}

func (r *PostgresRepository) ListTickets(ctx context.Context, accountID *uuid.UUID, status string, limit, offset int) ([]Ticket, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	status = strings.TrimSpace(status)
	rows, err := r.pool.Query(ctx, `
		SELECT t.id::text, t.ticket_number, t.category, t.subject, t.status,
		       t.assigned_to, t.discord_sync_status, t.created_at, t.updated_at, t.closed_at,
		       a.id::text, COALESCE(a.username, ''), COALESCE(a.email_ciphertext, t.requester_email_ciphertext)
		FROM support_tickets t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE ($1::uuid IS NULL OR t.account_id = $1)
		  AND ($2 = '' OR t.status = $2)
		ORDER BY t.latest_message_at DESC, t.created_at DESC
		LIMIT $3 OFFSET $4
	`, accountID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list support tickets: %w", err)
	}
	defer rows.Close()
	result := make([]Ticket, 0)
	for rows.Next() {
		ticket, err := r.scanTicket(rows, accountID == nil)
		if err != nil {
			return nil, fmt.Errorf("scan support ticket: %w", err)
		}
		if accountID != nil {
			ticket.AssignedTo = nil
		}
		result = append(result, ticket)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) GetTicket(ctx context.Context, accountID *uuid.UUID, ticketID uuid.UUID) (TicketDetail, error) {
	ticket, err := r.findTicket(ctx, r.pool, accountID, ticketID)
	if err != nil {
		return TicketDetail{}, err
	}
	if accountID != nil {
		ticket.AssignedTo = nil
		ticket.Requester = nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, author_type, source_platform, body, created_at, edited_at, status
		FROM support_messages
		WHERE ticket_id = $1
		ORDER BY created_at, id
	`, ticketID)
	if err != nil {
		return TicketDetail{}, fmt.Errorf("list support messages: %w", err)
	}
	defer rows.Close()
	messages := make([]Message, 0)
	for rows.Next() {
		message, err := scanSupportMessage(rows)
		if err != nil {
			return TicketDetail{}, fmt.Errorf("scan support message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return TicketDetail{}, err
	}
	attachments, err := r.loadAttachments(ctx, ticketID)
	if err != nil {
		return TicketDetail{}, err
	}
	for index := range messages {
		messages[index].Attachments = attachments[messages[index].ID]
	}
	return TicketDetail{Ticket: ticket, Messages: messages}, nil
}

func (r *PostgresRepository) AddUserMessage(ctx context.Context, accountID, ticketID uuid.UUID, body string) (Message, error) {
	return r.AddUserMessageWithAttachments(ctx, accountID, ticketID, body, nil)
}

func (r *PostgresRepository) AddUserMessageWithAttachments(ctx context.Context, accountID, ticketID uuid.UUID, body string, attachments []AttachmentInput) (Message, error) {
	if err := ValidateMessageBody(body); err != nil {
		return Message{}, err
	}
	if err := ValidateAttachments(attachments); err != nil {
		return Message{}, err
	}
	body = strings.TrimSpace(body)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback(ctx)
	ticketContext, err := lockTicket(ctx, tx, ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, err
	}
	if ticketContext.AccountID == nil || *ticketContext.AccountID != accountID {
		return Message{}, ErrForbidden
	}
	if ticketContext.Status == string(StatusClosed) || ticketContext.Status == string(StatusSpam) {
		return Message{}, ErrConflict
	}
	message, err := insertSupportMessage(ctx, tx, ticketID, &accountID, "user", string(SourceWebsite), nil, body, time.Now().UTC())
	if err != nil {
		return Message{}, err
	}
	message.Attachments, err = insertSupportAttachments(ctx, tx, ticketID, message.ID, attachments)
	if err != nil {
		return Message{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'waiting_staff', latest_message_at = CURRENT_TIMESTAMP WHERE id = $1`, ticketID); err != nil {
		return Message{}, err
	}
	if err := addTicketEvent(ctx, tx, ticketID, &accountID, "user_message", map[string]any{"message_id": message.ID.String()}); err != nil {
		return Message{}, err
	}
	if err := enqueueDiscordMessageTx(ctx, tx, ticketContext, message, "user"); err != nil {
		return Message{}, err
	}
	ticket := ticketContext.ticket()
	if err := r.enqueueOfficialEmailTx(ctx, tx, ticket, "message", "客服 Ticket "+ticket.TicketNumber+" 有新訊息", formatOfficialBody(ticket, body)); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (r *PostgresRepository) AddAdminMessage(ctx context.Context, adminID, ticketID uuid.UUID, body string) (Message, error) {
	return r.AddAdminMessageWithAttachments(ctx, adminID, ticketID, body, nil)
}

func (r *PostgresRepository) AddAdminMessageWithAttachments(ctx context.Context, adminID, ticketID uuid.UUID, body string, attachments []AttachmentInput) (Message, error) {
	if err := ValidateMessageBody(body); err != nil {
		return Message{}, err
	}
	if err := ValidateAttachments(attachments); err != nil {
		return Message{}, err
	}
	body = strings.TrimSpace(body)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback(ctx)
	if !isAdminTx(ctx, tx, adminID) {
		return Message{}, ErrAdminRequired
	}
	ticketContext, err := lockTicket(ctx, tx, ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, err
	}
	if ticketContext.Status == string(StatusClosed) || ticketContext.Status == string(StatusSpam) {
		return Message{}, ErrConflict
	}
	message, err := insertSupportMessage(ctx, tx, ticketID, &adminID, "admin", string(SourceWebsite), nil, body, time.Now().UTC())
	if err != nil {
		return Message{}, err
	}
	message.Attachments, err = insertSupportAttachments(ctx, tx, ticketID, message.ID, attachments)
	if err != nil {
		return Message{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'waiting_user', latest_message_at = CURRENT_TIMESTAMP WHERE id = $1`, ticketID); err != nil {
		return Message{}, err
	}
	if err := addTicketEvent(ctx, tx, ticketID, &adminID, "admin_message", map[string]any{"message_id": message.ID.String()}); err != nil {
		return Message{}, err
	}
	if err := enqueueDiscordMessageTx(ctx, tx, ticketContext, message, "admin"); err != nil {
		return Message{}, err
	}
	ticket := ticketContext.ticket()
	if err := r.enqueueUserNoticeTx(ctx, tx, ticket, "message:"+message.ID.String(), "客服已回覆", "客服已回覆你的 Ticket "+ticket.TicketNumber+"，請登入平台查看。", ticketContext.AccountID); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (r *PostgresRepository) SetTicketStatus(ctx context.Context, actorID, ticketID uuid.UUID, status TicketStatus, assignedTo *uuid.UUID, admin bool) (Ticket, error) {
	if err := ValidateStatus(status, admin); err != nil {
		return Ticket{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Ticket{}, err
	}
	defer tx.Rollback(ctx)
	if admin && !isAdminTx(ctx, tx, actorID) {
		return Ticket{}, ErrAdminRequired
	}
	ticketContext, err := lockTicket(ctx, tx, ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, err
	}
	if !admin && (ticketContext.AccountID == nil || *ticketContext.AccountID != actorID) {
		return Ticket{}, ErrForbidden
	}
	if !admin && ticketContext.Status == string(StatusSpam) {
		return Ticket{}, ErrConflict
	}
	if !admin && status == StatusClosed && ticketContext.Status == string(StatusClosed) {
		return ticketContext.ticket(), nil
	}
	if assignedTo != nil {
		if !isAdminTx(ctx, tx, *assignedTo) {
			return Ticket{}, ErrForbidden
		}
	}
	if status == StatusClosed {
		_, err = tx.Exec(ctx, `
			UPDATE support_tickets
			SET status = 'closed', closed_at = COALESCE(closed_at, CURRENT_TIMESTAMP), latest_message_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`, ticketID)
	} else if admin && assignedTo != nil {
		_, err = tx.Exec(ctx, `
			UPDATE support_tickets
			SET status = $2, assigned_to = $3, closed_at = NULL, latest_message_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`, ticketID, status, *assignedTo)
	} else if admin && assignedTo == nil {
		_, err = tx.Exec(ctx, `
			UPDATE support_tickets
			SET status = $2, closed_at = NULL, latest_message_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`, ticketID, status)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE support_tickets
			SET status = $2, closed_at = NULL, latest_message_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`, ticketID, status)
	}
	if err != nil {
		return Ticket{}, err
	}
	if err := addTicketEvent(ctx, tx, ticketID, &actorID, "status_changed", map[string]any{
		"from": ticketContext.Status,
		"to":   string(status),
	}); err != nil {
		return Ticket{}, err
	}
	updated := ticketContext.ticket()
	updated.Status = string(status)
	if status == StatusClosed {
		now := time.Now().UTC()
		updated.ClosedAt = &now
		if err := enqueueDiscordArchiveTx(ctx, tx, ticketContext); err != nil {
			return Ticket{}, err
		}
		if err := r.enqueueUserNoticeTx(ctx, tx, updated, "closed", "客服單已結束", "你的 Ticket "+updated.TicketNumber+" 已結束；如需追問，請建立新的客服單。", ticketContext.AccountID); err != nil {
			return Ticket{}, err
		}
	} else if ticketContext.Status == string(StatusClosed) {
		if err := enqueueDiscordReopenTx(ctx, tx, ticketContext); err != nil {
			return Ticket{}, err
		}
	} else if !admin && status == StatusWaitingStaff {
		if err := r.enqueueOfficialEmailTx(ctx, tx, updated, "reopened", "客服 Ticket "+updated.TicketNumber+" 已重新開啟", "使用者已重新開啟 Ticket，請登入客服後台處理。\n\n查看連結："+r.ticketURL(updated.ID)); err != nil {
			return Ticket{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Ticket{}, err
	}
	return updated, nil
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) ApplyDiscordMessage(ctx context.Context, input ExternalMessage, authorHashKey []byte) (Message, error) {
	if err := ValidateExternalMessage(input); err != nil {
		return Message{}, err
	}
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	input.ExternalMessageID = strings.TrimSpace(input.ExternalMessageID)
	input.ExternalAuthorID = strings.TrimSpace(input.ExternalAuthorID)
	input.Body = strings.TrimSpace(input.Body)
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback(ctx)
	contextRow, err := lockTicketByDiscordChannel(ctx, tx, input.ChannelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, err
	}
	if contextRow.Status == string(StatusClosed) || contextRow.Status == string(StatusSpam) {
		return Message{}, ErrConflict
	}
	if input.Operation == OperationCreate {
		var existingID uuid.UUID
		err = tx.QueryRow(ctx, `SELECT message_id FROM support_message_bridges WHERE platform = 'discord' AND external_message_id = $1`, input.ExternalMessageID).Scan(&existingID)
		if err == nil {
			message, getErr := getSupportMessage(ctx, tx, existingID)
			if getErr != nil {
				return Message{}, getErr
			}
			if err := tx.Commit(ctx); err != nil {
				return Message{}, err
			}
			return message, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Message{}, err
		}
		externalHash := []byte(nil)
		if len(authorHashKey) == 32 {
			externalHash, err = auth.LookupHash(authorHashKey, input.ExternalAuthorID)
			if err != nil {
				return Message{}, err
			}
		}
		message, err := insertSupportMessage(ctx, tx, contextRow.ID, nil, "admin", string(SourceDiscord), externalHash, input.Body, input.CreatedAt)
		if err != nil {
			return Message{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_message_bridges (message_id, platform, external_message_id, sync_status, synced_at) VALUES ($1, 'discord', $2, 'sent', CURRENT_TIMESTAMP)`, message.ID, input.ExternalMessageID); err != nil {
			return Message{}, mapSupportError(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_discord_inbound_dedup (external_message_id, message_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, input.ExternalMessageID, message.ID); err != nil {
			return Message{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'waiting_user', latest_message_at = CURRENT_TIMESTAMP WHERE id = $1`, contextRow.ID); err != nil {
			return Message{}, err
		}
		if err := addTicketEvent(ctx, tx, contextRow.ID, nil, "discord_message", map[string]any{"message_id": message.ID.String()}); err != nil {
			return Message{}, err
		}
		ticket := contextRow.ticket()
		if err := r.enqueueUserNoticeTx(ctx, tx, ticket, "discord-message:"+message.ID.String(), "客服已回覆", "客服已在客服頻道回覆你的 Ticket "+ticket.TicketNumber+"，請登入平台查看。", contextRow.AccountID); err != nil {
			return Message{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Message{}, err
		}
		return message, nil
	}
	var messageID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT message_id FROM support_message_bridges WHERE platform = 'discord' AND external_message_id = $1`, input.ExternalMessageID).Scan(&messageID); errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	} else if err != nil {
		return Message{}, err
	}
	var message Message
	var messageIDText string
	if input.Operation == OperationDelete {
		err = tx.QueryRow(ctx, `UPDATE support_messages SET body = '', status = 'deleted', deleted_at = CURRENT_TIMESTAMP WHERE id = $1 AND ticket_id = $2 RETURNING id::text, author_type, source_platform, body, created_at, edited_at, status`, messageID, contextRow.ID).Scan(&messageIDText, &message.AuthorType, &message.SourcePlatform, &message.Body, &message.CreatedAt, &message.EditedAt, &message.Status)
	} else {
		err = tx.QueryRow(ctx, `UPDATE support_messages SET body = $2, status = 'edited', edited_at = CURRENT_TIMESTAMP WHERE id = $1 AND ticket_id = $3 RETURNING id::text, author_type, source_platform, body, created_at, edited_at, status`, messageID, input.Body, contextRow.ID).Scan(&messageIDText, &message.AuthorType, &message.SourcePlatform, &message.Body, &message.CreatedAt, &message.EditedAt, &message.Status)
	}
	if err != nil {
		return Message{}, mapSupportError(err)
	}
	message.ID, err = uuid.Parse(messageIDText)
	if err != nil {
		return Message{}, err
	}
	bridgeStatus := "edited"
	if input.Operation == OperationDelete {
		bridgeStatus = "deleted"
	}
	if _, err := tx.Exec(ctx, `UPDATE support_message_bridges SET sync_status = $2, synced_at = CURRENT_TIMESTAMP WHERE message_id = $1 AND platform = 'discord'`, messageID, bridgeStatus); err != nil {
		return Message{}, err
	}
	if err := addTicketEvent(ctx, tx, contextRow.ID, nil, "discord_message_"+string(input.Operation), map[string]any{"message_id": messageID.String()}); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (r *PostgresRepository) ApplyEmailMessage(ctx context.Context, input ExternalEmailMessage, authorHashKey []byte) (Message, error) {
	if err := ValidateExternalEmailMessage(input); err != nil {
		return Message{}, err
	}
	input.ExternalMessageID = strings.TrimSpace(input.ExternalMessageID)
	input.TicketNumber = strings.TrimSpace(input.TicketNumber)
	input.From = strings.TrimSpace(input.From)
	input.Body = strings.TrimSpace(input.Body)
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	ticketNumber, err := parseTicketNumberValue(input.TicketNumber)
	if err != nil {
		return Message{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback(ctx)
	ticketContext, err := lockTicketByNumber(ctx, tx, ticketNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, err
	}
	if ticketContext.Status == string(StatusClosed) || ticketContext.Status == string(StatusSpam) {
		return Message{}, ErrConflict
	}
	var existingID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT message_id FROM support_email_inbound_dedup WHERE external_message_id = $1`, input.ExternalMessageID).Scan(&existingID); err == nil {
		message, getErr := getSupportMessage(ctx, tx, existingID)
		if getErr != nil {
			return Message{}, getErr
		}
		if err := tx.Commit(ctx); err != nil {
			return Message{}, err
		}
		return message, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Message{}, err
	}
	var externalHash []byte
	if len(authorHashKey) == 32 {
		externalHash, err = auth.LookupHash(authorHashKey, input.From)
		if err != nil {
			return Message{}, err
		}
	}
	message, err := insertSupportMessage(ctx, tx, ticketContext.ID, nil, "admin", string(SourceEmail), externalHash, input.Body, input.CreatedAt)
	if err != nil {
		return Message{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO support_email_inbound_dedup (external_message_id, message_id) VALUES ($1, $2)`, input.ExternalMessageID, message.ID); err != nil {
		return Message{}, mapSupportError(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'waiting_user', latest_message_at = CURRENT_TIMESTAMP WHERE id = $1`, ticketContext.ID); err != nil {
		return Message{}, err
	}
	if err := addTicketEvent(ctx, tx, ticketContext.ID, nil, "email_message", map[string]any{"message_id": message.ID.String()}); err != nil {
		return Message{}, err
	}
	if err := enqueueDiscordMessageTx(ctx, tx, ticketContext, message, "admin"); err != nil {
		return Message{}, err
	}
	if err := r.enqueueUserNoticeTx(ctx, tx, ticketContext.ticket(), "email-message:"+message.ID.String(), "客服已回覆", "客服已透過 Email 回覆你的 Ticket "+ticketContext.ticket().TicketNumber+"，請登入平台查看。", ticketContext.AccountID); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (r *PostgresRepository) ClaimDiscordOutbox(ctx context.Context, limit int) ([]DiscordOutboxTask, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT o.id::text, o.ticket_id::text, o.message_id::text, t.ticket_number,
		       t.subject, COALESCE(t.discord_channel_id, ''), o.operation,
		       COALESCE(m.body, ''), COALESCE(o.payload->>'author_type', ''),
		       COALESCE(o.external_message_id, '')
		FROM support_discord_outbox o
		JOIN support_tickets t ON t.id = o.ticket_id
		LEFT JOIN support_messages m ON m.id = o.message_id
		WHERE ((o.status IN ('pending', 'failed') AND o.available_at <= CURRENT_TIMESTAMP)
		   OR (o.status = 'processing' AND o.updated_at < CURRENT_TIMESTAMP - INTERVAL '5 minutes'))
		  AND NOT EXISTS (
			SELECT 1 FROM support_discord_outbox previous
			WHERE previous.ticket_id = o.ticket_id
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
	tasks := make([]DiscordOutboxTask, 0, limit)
	for rows.Next() {
		var task DiscordOutboxTask
		var idText, ticketIDText, messageIDText string
		if err := rows.Scan(&idText, &ticketIDText, &messageIDText, &task.TicketNumber, &task.Subject, &task.ChannelID, &task.Operation, &task.Body, &task.AuthorType, &task.ExternalMessageID); err != nil {
			rows.Close()
			return nil, err
		}
		if task.ID, err = uuid.Parse(idText); err != nil {
			rows.Close()
			return nil, err
		}
		if task.TicketID, err = uuid.Parse(ticketIDText); err != nil {
			rows.Close()
			return nil, err
		}
		if messageIDText != "" {
			id, parseErr := uuid.Parse(messageIDText)
			if parseErr != nil {
				rows.Close()
				return nil, parseErr
			}
			task.MessageID = &id
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, task := range tasks {
		if _, err := tx.Exec(ctx, `UPDATE support_discord_outbox SET status = 'processing', attempt_count = attempt_count + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, task.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *PostgresRepository) MarkDiscordOutboxSent(ctx context.Context, task DiscordOutboxTask, externalID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if task.Operation == "create_channel" {
		if strings.TrimSpace(externalID) == "" {
			return errors.New("created Discord channel id is missing")
		}
		if _, err := tx.Exec(ctx, `UPDATE support_tickets SET discord_channel_id = $2, discord_sync_status = 'synced' WHERE id = $1`, task.TicketID, externalID); err != nil {
			return err
		}
	} else if task.Operation == "create_message" {
		if task.MessageID == nil || strings.TrimSpace(externalID) == "" {
			return errors.New("created Discord message data is missing")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO support_message_bridges (message_id, platform, external_message_id, sync_status, synced_at)
			VALUES ($1, 'discord', $2, 'sent', CURRENT_TIMESTAMP)
			ON CONFLICT (message_id, platform) DO UPDATE SET external_message_id = EXCLUDED.external_message_id, sync_status = 'sent', synced_at = CURRENT_TIMESTAMP, last_error = NULL
		`, *task.MessageID, externalID); err != nil {
			return mapSupportError(err)
		}
	} else if task.Operation == "archive_channel" || task.Operation == "reopen_channel" {
		if _, err := tx.Exec(ctx, `UPDATE support_tickets SET discord_sync_status = 'archived' WHERE id = $1`, task.TicketID); err != nil {
			return err
		}
		if task.Operation == "reopen_channel" {
			if _, err := tx.Exec(ctx, `UPDATE support_tickets SET discord_sync_status = 'synced' WHERE id = $1`, task.TicketID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE support_discord_outbox SET status = 'sent', last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, task.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) MarkDiscordOutboxFailed(ctx context.Context, task DiscordOutboxTask, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE support_discord_outbox
		SET status = CASE WHEN attempt_count >= max_attempts THEN 'abandoned' ELSE 'failed' END,
		    last_error = $2,
		    available_at = CURRENT_TIMESTAMP + (INTERVAL '30 seconds' * LEAST(GREATEST(attempt_count, 1), 10)),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, task.ID, message)
	return err
}

type supportTicketContext struct {
	ID                       uuid.UUID
	TicketNumber             int64
	AccountID                *uuid.UUID
	RequesterEmailCiphertext []byte
	Category                 string
	Subject                  string
	Status                   string
	AssignedTo               *uuid.UUID
	DiscordChannelID         string
	DiscordSyncStatus        string
	CreatedAt                time.Time
	UpdatedAt                time.Time
	ClosedAt                 *time.Time
}

func (c supportTicketContext) ticket() Ticket {
	return Ticket{
		ID: c.ID, TicketNumber: ticketNumber(c.TicketNumber), Category: c.Category,
		Subject: c.Subject, Status: c.Status, AssignedTo: c.AssignedTo,
		DiscordSyncStatus: c.DiscordSyncStatus, CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt, ClosedAt: c.ClosedAt,
	}
}

func insertTicket(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, input CreateTicketInput) (Ticket, error) {
	var row supportTicketContext
	var idText string
	err := tx.QueryRow(ctx, `
		INSERT INTO support_tickets (account_id, category, subject, status, latest_message_at)
		VALUES ($1, $2, $3, 'waiting_staff', CURRENT_TIMESTAMP)
		RETURNING id::text, ticket_number, category, subject, status, assigned_to,
		          discord_sync_status, created_at, updated_at, closed_at
	`, accountID, input.Category, input.Subject).Scan(
		&idText, &row.TicketNumber, &row.Category, &row.Subject, &row.Status,
		&row.AssignedTo, &row.DiscordSyncStatus, &row.CreatedAt, &row.UpdatedAt, &row.ClosedAt,
	)
	if err != nil {
		return Ticket{}, err
	}
	row.ID, err = uuid.Parse(idText)
	if err != nil {
		return Ticket{}, err
	}
	return row.ticket(), nil
}

func (r *PostgresRepository) findTicket(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, accountID *uuid.UUID, ticketID uuid.UUID) (Ticket, error) {
	ticket, err := r.scanTicket(querier.QueryRow(ctx, `
		SELECT t.id::text, t.ticket_number, t.category, t.subject, t.status, t.assigned_to,
		       t.discord_sync_status, t.created_at, t.updated_at, t.closed_at,
		       a.id::text, COALESCE(a.username, ''), COALESCE(a.email_ciphertext, t.requester_email_ciphertext)
		FROM support_tickets t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.id = $1 AND ($2::uuid IS NULL OR t.account_id = $2)
	`, ticketID, accountID), accountID == nil)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func lockTicket(ctx context.Context, tx pgx.Tx, ticketID uuid.UUID) (supportTicketContext, error) {
	var row supportTicketContext
	var idText string
	err := tx.QueryRow(ctx, `
		SELECT id::text, ticket_number, account_id, requester_email_ciphertext, category, subject,
		       status, assigned_to, COALESCE(discord_channel_id, ''), discord_sync_status,
		       created_at, updated_at, closed_at
		FROM support_tickets WHERE id = $1 FOR UPDATE
	`, ticketID).Scan(
		&idText, &row.TicketNumber, &row.AccountID, &row.RequesterEmailCiphertext, &row.Category, &row.Subject,
		&row.Status, &row.AssignedTo, &row.DiscordChannelID, &row.DiscordSyncStatus,
		&row.CreatedAt, &row.UpdatedAt, &row.ClosedAt,
	)
	if err != nil {
		return supportTicketContext{}, err
	}
	row.ID, err = uuid.Parse(idText)
	return row, err
}

func lockTicketByDiscordChannel(ctx context.Context, tx pgx.Tx, channelID string) (supportTicketContext, error) {
	var row supportTicketContext
	var idText string
	err := tx.QueryRow(ctx, `
		SELECT id::text, ticket_number, account_id, requester_email_ciphertext, category, subject,
		       status, assigned_to, COALESCE(discord_channel_id, ''), discord_sync_status,
		       created_at, updated_at, closed_at
		FROM support_tickets WHERE discord_channel_id = $1 FOR UPDATE
	`, channelID).Scan(
		&idText, &row.TicketNumber, &row.AccountID, &row.RequesterEmailCiphertext, &row.Category, &row.Subject,
		&row.Status, &row.AssignedTo, &row.DiscordChannelID, &row.DiscordSyncStatus,
		&row.CreatedAt, &row.UpdatedAt, &row.ClosedAt,
	)
	if err != nil {
		return supportTicketContext{}, err
	}
	row.ID, err = uuid.Parse(idText)
	return row, err
}

func lockTicketByNumber(ctx context.Context, tx pgx.Tx, number int64) (supportTicketContext, error) {
	var row supportTicketContext
	var idText string
	err := tx.QueryRow(ctx, `
		SELECT id::text, ticket_number, account_id, requester_email_ciphertext, category, subject,
		       status, assigned_to, COALESCE(discord_channel_id, ''), discord_sync_status,
		       created_at, updated_at, closed_at
		FROM support_tickets WHERE ticket_number = $1 FOR UPDATE
	`, number).Scan(
		&idText, &row.TicketNumber, &row.AccountID, &row.RequesterEmailCiphertext, &row.Category, &row.Subject,
		&row.Status, &row.AssignedTo, &row.DiscordChannelID, &row.DiscordSyncStatus,
		&row.CreatedAt, &row.UpdatedAt, &row.ClosedAt,
	)
	if err != nil {
		return supportTicketContext{}, err
	}
	row.ID, err = uuid.Parse(idText)
	return row, err
}

func insertSupportMessage(ctx context.Context, tx pgx.Tx, ticketID uuid.UUID, authorID *uuid.UUID, authorType, source string, externalAuthorHash []byte, body string, createdAt time.Time) (Message, error) {
	var message Message
	var idText string
	err := tx.QueryRow(ctx, `
		INSERT INTO support_messages (ticket_id, author_account_id, author_type, source_platform, external_author_hash, body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, author_type, source_platform, body, created_at, edited_at, status
	`, ticketID, authorID, authorType, source, externalAuthorHash, body, createdAt).Scan(
		&idText, &message.AuthorType, &message.SourcePlatform, &message.Body, &message.CreatedAt, &message.EditedAt, &message.Status,
	)
	if err != nil {
		return Message{}, mapSupportError(err)
	}
	message.ID, err = uuid.Parse(idText)
	return message, err
}

func insertSupportAttachments(ctx context.Context, tx pgx.Tx, ticketID, messageID uuid.UUID, inputs []AttachmentInput) ([]Attachment, error) {
	if err := ValidateAttachments(inputs); err != nil {
		return nil, err
	}
	result := make([]Attachment, 0, len(inputs))
	for _, input := range inputs {
		var attachment Attachment
		err := tx.QueryRow(ctx, `
			INSERT INTO support_attachments
				(ticket_id, message_id, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, ticket_id, message_id, original_file_name, mime_type, file_size_bytes, sha256_hex, created_at
		`, ticketID, messageID, input.StorageKey, input.OriginalName, input.MIMEType, input.FileSizeBytes, strings.ToLower(input.SHA256)).Scan(
			&attachment.ID, &attachment.TicketID, &attachment.MessageID, &attachment.OriginalName,
			&attachment.MIMEType, &attachment.FileSizeBytes, &attachment.SHA256, &attachment.CreatedAt,
		)
		if err != nil {
			return nil, mapSupportError(err)
		}
		attachment.storageKey = input.StorageKey
		result = append(result, attachment)
	}
	return result, nil
}

func (r *PostgresRepository) loadAttachments(ctx context.Context, ticketID uuid.UUID) (map[uuid.UUID][]Attachment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, ticket_id, message_id, storage_key, original_file_name, mime_type,
		       file_size_bytes, sha256_hex, created_at
		FROM support_attachments
		WHERE ticket_id = $1
		ORDER BY created_at, id
	`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list support attachments: %w", err)
	}
	defer rows.Close()
	result := make(map[uuid.UUID][]Attachment)
	for rows.Next() {
		attachment, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		result[attachment.MessageID] = append(result[attachment.MessageID], attachment)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) GetAttachment(ctx context.Context, accountID *uuid.UUID, ticketID, attachmentID uuid.UUID) (Attachment, error) {
	query := `
		SELECT a.id, a.ticket_id, a.message_id, a.storage_key, a.original_file_name,
		       a.mime_type, a.file_size_bytes, a.sha256_hex, a.created_at
		FROM support_attachments a
		JOIN support_tickets t ON t.id = a.ticket_id
		WHERE a.id = $1 AND a.ticket_id = $2
	`
	args := []any{attachmentID, ticketID}
	if accountID != nil {
		query += ` AND t.account_id = $3`
		args = append(args, *accountID)
	}
	attachment, err := scanAttachment(r.pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, err
	}
	return attachment, nil
}

func getSupportMessage(ctx context.Context, tx pgx.Tx, messageID uuid.UUID) (Message, error) {
	return scanSupportMessage(tx.QueryRow(ctx, `SELECT id::text, author_type, source_platform, body, created_at, edited_at, status FROM support_messages WHERE id = $1`, messageID))
}

func (r *PostgresRepository) scanTicket(row interface{ Scan(...any) error }, includeRequester bool) (Ticket, error) {
	var ticket Ticket
	var idText string
	var number int64
	var requesterIDText *string
	var requesterUsername string
	var requesterEmailCiphertext []byte
	if err := row.Scan(&idText, &number, &ticket.Category, &ticket.Subject, &ticket.Status, &ticket.AssignedTo, &ticket.DiscordSyncStatus, &ticket.CreatedAt, &ticket.UpdatedAt, &ticket.ClosedAt, &requesterIDText, &requesterUsername, &requesterEmailCiphertext); err != nil {
		return Ticket{}, err
	}
	var err error
	ticket.ID, err = uuid.Parse(idText)
	if err != nil {
		return Ticket{}, err
	}
	ticket.TicketNumber = ticketNumber(number)
	if includeRequester {
		requester := &Requester{Username: requesterUsername}
		if requesterIDText != nil && strings.TrimSpace(*requesterIDText) != "" {
			parsedID, parseErr := uuid.Parse(*requesterIDText)
			err = parseErr
			if err != nil {
				return Ticket{}, err
			}
			requester.AccountID = &parsedID
		}
		if len(requesterEmailCiphertext) > 0 {
			requester.Email, err = r.cipher.Open(requesterEmailCiphertext)
			if err != nil {
				return Ticket{}, err
			}
		}
		if requester.AccountID != nil || requester.Username != "" || requester.Email != "" {
			ticket.Requester = requester
		}
	}
	return ticket, nil
}

func scanSupportMessage(row interface{ Scan(...any) error }) (Message, error) {
	var message Message
	var idText string
	if err := row.Scan(&idText, &message.AuthorType, &message.SourcePlatform, &message.Body, &message.CreatedAt, &message.EditedAt, &message.Status); err != nil {
		return Message{}, err
	}
	var err error
	message.ID, err = uuid.Parse(idText)
	return message, err
}

func scanAttachment(row interface{ Scan(...any) error }) (Attachment, error) {
	var attachment Attachment
	if err := row.Scan(&attachment.ID, &attachment.TicketID, &attachment.MessageID, &attachment.storageKey, &attachment.OriginalName, &attachment.MIMEType, &attachment.FileSizeBytes, &attachment.SHA256, &attachment.CreatedAt); err != nil {
		return Attachment{}, err
	}
	return attachment, nil
}

func isAdminTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) bool {
	var exists bool
	return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists) == nil && exists
}

func addTicketEvent(ctx context.Context, tx pgx.Tx, ticketID uuid.UUID, actorID *uuid.UUID, eventType string, data map[string]any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO support_ticket_events (ticket_id, actor_account_id, event_type, data) VALUES ($1, $2, $3, $4)`, ticketID, actorID, eventType, payload)
	return err
}

func enqueueDiscordChannelTx(ctx context.Context, tx pgx.Tx, ticketID uuid.UUID, number string, subject string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO support_discord_outbox (ticket_id, operation, payload)
		VALUES ($1, 'create_channel', jsonb_build_object('ticket_number', $2::text, 'subject', $3::text))
	`, ticketID, number, subject)
	return err
}

func enqueueDiscordMessageTx(ctx context.Context, tx pgx.Tx, ticket supportTicketContext, message Message, authorType string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO support_discord_outbox (ticket_id, message_id, operation, payload)
		VALUES ($1, $2, 'create_message', jsonb_build_object('body', $3::text, 'author_type', $4::text))
	`, ticket.ID, message.ID, message.Body, authorType)
	return err
}

func enqueueDiscordArchiveTx(ctx context.Context, tx pgx.Tx, ticket supportTicketContext) error {
	_, err := tx.Exec(ctx, `INSERT INTO support_discord_outbox (ticket_id, operation, payload) VALUES ($1, 'archive_channel', '{}'::jsonb)`, ticket.ID)
	return err
}

func enqueueDiscordReopenTx(ctx context.Context, tx pgx.Tx, ticket supportTicketContext) error {
	_, err := tx.Exec(ctx, `INSERT INTO support_discord_outbox (ticket_id, operation, payload) VALUES ($1, 'reopen_channel', '{}'::jsonb)`, ticket.ID)
	return err
}

func (r *PostgresRepository) enqueueOfficialEmailTx(ctx context.Context, tx pgx.Tx, ticket Ticket, suffix, subject, body string) error {
	if r.supportEmail == "" {
		return nil
	}
	recipient, err := r.cipher.Seal(r.supportEmail)
	if err != nil {
		return err
	}
	return enqueueEmailTx(ctx, tx, r.cipher, nil, recipient, "support:"+ticket.ID.String()+":"+suffix, subject, body)
}

func (r *PostgresRepository) enqueueUserNoticeTx(ctx context.Context, tx pgx.Tx, ticket Ticket, suffix, title, body string, accountID *uuid.UUID) error {
	var recipient []byte
	if accountID != nil {
		if err := tx.QueryRow(ctx, `SELECT email_ciphertext FROM accounts WHERE id = $1 AND account_status = 'active'`, *accountID).Scan(&recipient); err != nil {
			return err
		}
	} else {
		var stored []byte
		if err := tx.QueryRow(ctx, `SELECT requester_email_ciphertext FROM support_tickets WHERE id = $1`, ticket.ID).Scan(&stored); err != nil {
			return err
		}
		recipient = stored
	}
	if accountID != nil {
		titleCiphertext, err := r.cipher.Seal(title)
		if err != nil {
			return err
		}
		bodyCiphertext, err := r.cipher.Seal(body)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO notifications (account_id, kind, dedup_key, title_ciphertext, body_ciphertext)
			VALUES ($1, 'support', $2, $3, $4)
			ON CONFLICT (account_id, dedup_key) DO NOTHING
		`, *accountID, "support:"+ticket.ID.String()+":"+suffix, titleCiphertext, bodyCiphertext); err != nil {
			return err
		}
	}
	return enqueueEmailTx(ctx, tx, r.cipher, accountID, recipient, "support:"+ticket.ID.String()+":"+suffix, title, body+"\n\n查看連結："+r.ticketURL(ticket.ID))
}

func enqueueEmailTx(ctx context.Context, tx pgx.Tx, cipher *auth.FieldCipher, accountID *uuid.UUID, recipientCiphertext []byte, dedupKey, subject, body string) error {
	if len(recipientCiphertext) == 0 || strings.TrimSpace(dedupKey) == "" || strings.TrimSpace(subject) == "" || strings.TrimSpace(body) == "" {
		return nil
	}
	payload, err := json.Marshal(struct {
		Subject string `json:"subject"`
		Text    string `json:"text"`
	}{Subject: subject, Text: body})
	if err != nil {
		return err
	}
	payloadCiphertext, err := cipher.Seal(string(payload))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO email_outbox (account_id, notification_id, dedup_key, recipient_ciphertext, payload_ciphertext)
		VALUES ($1, NULL, $2, $3, $4)
		ON CONFLICT DO NOTHING
	`, accountID, dedupKey, recipientCiphertext, payloadCiphertext)
	return err
}

func (r *PostgresRepository) ticketURL(ticketID uuid.UUID) string {
	if r.publicBaseURL == "" {
		return "/support/tickets/" + ticketID.String()
	}
	return r.publicBaseURL + "/support/tickets/" + ticketID.String()
}

func formatOfficialBody(ticket Ticket, body string) string {
	return "Ticket：" + ticket.TicketNumber + "\n" +
		"分類：" + ticket.Category + "\n" +
		"主旨：" + ticket.Subject + "\n\n" +
		body + "\n"
}

func mapSupportError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("support database operation: %w", err)
}
