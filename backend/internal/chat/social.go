package chat

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"sta-backend/internal/pagination"
)

// messageSelect is the column list shared by every message-list query. Column
// order must match scanMessage.
const messageSelect = `m.id::text, m.body, m.source_platform, m.status, m.created_at, m.edited_at,
	m.parent_message_id, m.pinned_at, c.channel_key`

func scanMessage(rows pgx.Row) (Message, error) {
	var m Message
	var idText, channelKey string
	if err := rows.Scan(&idText, &m.Body, &m.SourcePlatform, &m.Status, &m.CreatedAt, &m.EditedAt,
		&m.ParentID, &m.PinnedAt, &channelKey); err != nil {
		return Message{}, err
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return Message{}, err
	}
	m.ID = id
	m.ChannelKey = channelKey
	return m, nil
}

func (r *PostgresRepository) ListChannels(ctx context.Context) ([]Channel, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT channel_key, display_name, kind, COALESCE(topic, ''), is_default
		FROM chat_channels
		WHERE is_active AND archived_at IS NULL
		ORDER BY is_default DESC, display_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := make([]Channel, 0, 8)
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.Key, &c.DisplayName, &c.Kind, &c.Topic, &c.IsDefault); err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

// channelID resolves an active, non-archived channel by key and reports whether
// it is the bridged default channel.
func (r *PostgresRepository) channelID(ctx context.Context, channelKey string) (uuid.UUID, bool, error) {
	var id uuid.UUID
	var isDefault bool
	err := r.pool.QueryRow(ctx, `
		SELECT id, is_default FROM chat_channels
		WHERE channel_key = $1 AND is_active AND archived_at IS NULL`, channelKey).Scan(&id, &isDefault)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, isDefault, nil
}

func (r *PostgresRepository) ListChannelMessages(ctx context.Context, channelKey string, viewer uuid.UUID, limit int, after pagination.Cursor) ([]Message, string, error) {
	limit = pagination.ClampLimit(limit, 50, 100)
	if _, _, err := r.channelID(ctx, channelKey); err != nil {
		return nil, "", err
	}
	var afterTime *time.Time
	var afterID *uuid.UUID
	if !after.Zero() {
		t := after.Time
		id := after.UUID()
		afterTime, afterID = &t, &id
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+messageSelect+`
		FROM chat_messages m
		JOIN chat_channels c ON c.id = m.channel_id
		WHERE c.channel_key = $4
		  AND m.status <> 'deleted'
		  AND m.parent_message_id IS NULL
		  AND ($2::timestamptz IS NULL OR (m.created_at, m.id) < ($2::timestamptz, $3::uuid))
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $1
	`, limit, afterTime, afterID, channelKey)
	if err != nil {
		return nil, "", err
	}
	result, err := collectMessages(rows)
	if err != nil {
		return nil, "", err
	}
	if err := r.enrich(ctx, result, viewer); err != nil {
		return nil, "", err
	}
	var next string
	if n := len(result); n > 0 {
		last := result[n-1]
		next = pagination.Next(n, limit, last.CreatedAt, last.ID)
	}
	return result, next, nil
}

func (r *PostgresRepository) ListThreadReplies(ctx context.Context, parentID, viewer uuid.UUID, limit int, after pagination.Cursor) ([]Message, string, error) {
	limit = pagination.ClampLimit(limit, 50, 100)
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM chat_messages WHERE id = $1)`, parentID).Scan(&exists); err != nil {
		return nil, "", err
	}
	if !exists {
		return nil, "", ErrNotFound
	}
	var afterTime *time.Time
	var afterID *uuid.UUID
	if !after.Zero() {
		t := after.Time
		id := after.UUID()
		afterTime, afterID = &t, &id
	}
	// Replies read oldest-first: a thread is a short conversation.
	rows, err := r.pool.Query(ctx, `
		SELECT `+messageSelect+`
		FROM chat_messages m
		JOIN chat_channels c ON c.id = m.channel_id
		WHERE m.parent_message_id = $4
		  AND m.status <> 'deleted'
		  AND ($2::timestamptz IS NULL OR (m.created_at, m.id) > ($2::timestamptz, $3::uuid))
		ORDER BY m.created_at ASC, m.id ASC
		LIMIT $1
	`, limit, afterTime, afterID, parentID)
	if err != nil {
		return nil, "", err
	}
	result, err := collectMessages(rows)
	if err != nil {
		return nil, "", err
	}
	if err := r.enrich(ctx, result, viewer); err != nil {
		return nil, "", err
	}
	var next string
	if n := len(result); n > 0 {
		last := result[n-1]
		next = pagination.Next(n, limit, last.CreatedAt, last.ID)
	}
	return result, next, nil
}

func (r *PostgresRepository) ListPinned(ctx context.Context, channelKey string, viewer uuid.UUID) ([]Message, error) {
	if _, _, err := r.channelID(ctx, channelKey); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+messageSelect+`
		FROM chat_messages m
		JOIN chat_channels c ON c.id = m.channel_id
		WHERE c.channel_key = $1 AND m.pinned_at IS NOT NULL AND m.status <> 'deleted'
		ORDER BY m.pinned_at DESC
		LIMIT 100
	`, channelKey)
	if err != nil {
		return nil, err
	}
	result, err := collectMessages(rows)
	if err != nil {
		return nil, err
	}
	if err := r.enrich(ctx, result, viewer); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *PostgresRepository) CreateChannelMessage(ctx context.Context, channelKey string, accountID uuid.UUID, body string, parentID *uuid.UUID) (Message, error) {
	channelID, isDefault, err := r.channelID(ctx, channelKey)
	if err != nil {
		return Message{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback(ctx)

	if parentID != nil {
		// The parent must exist, sit in the same channel and not itself be a
		// reply — threads are one level deep.
		var parentChannel uuid.UUID
		var parentHasParent bool
		err := tx.QueryRow(ctx, `
			SELECT channel_id, parent_message_id IS NOT NULL
			FROM chat_messages WHERE id = $1 AND status <> 'deleted'`, *parentID).Scan(&parentChannel, &parentHasParent)
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrNotFound
		}
		if err != nil {
			return Message{}, err
		}
		if parentChannel != channelID || parentHasParent {
			return Message{}, ErrInvalidMessage
		}
	}

	message, err := insertMessage(ctx, tx, channelID, &accountID, PlatformWebsite, nil, body, parentID)
	if err != nil {
		return Message{}, err
	}
	// Only the default channel is bridged to Discord/Telegram, and thread
	// replies stay in-app.
	if isDefault && parentID == nil {
		if err := createOutboundTasks(ctx, tx, message.ID, OperationCreate, message.Body); err != nil {
			return Message{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	message.ChannelKey = channelKey
	// Live-stream top-level messages on every channel; thread replies are
	// fetched on demand.
	if parentID == nil {
		r.announce(ctx, channelKey, message)
	}
	return message, nil
}

// editableMessage loads a message for an owner-only mutation, taking a row lock.
type editableMessage struct {
	channelKey string
	isDefault  bool
}

func lockOwnedWebsiteMessage(ctx context.Context, tx pgx.Tx, messageID, accountID uuid.UUID) (editableMessage, error) {
	var (
		author   *uuid.UUID
		platform string
		status   string
		info     editableMessage
	)
	err := tx.QueryRow(ctx, `
		SELECT m.author_account_id, m.source_platform, m.status, c.channel_key, c.is_default
		FROM chat_messages m
		JOIN chat_channels c ON c.id = m.channel_id
		WHERE m.id = $1
		FOR UPDATE OF m`, messageID).Scan(&author, &platform, &status, &info.channelKey, &info.isDefault)
	if errors.Is(err, pgx.ErrNoRows) {
		return editableMessage{}, ErrNotFound
	}
	if err != nil {
		return editableMessage{}, err
	}
	if status == "deleted" {
		return editableMessage{}, ErrNotFound
	}
	if platform != string(PlatformWebsite) || author == nil || *author != accountID {
		return editableMessage{}, ErrForbidden
	}
	return info, nil
}

func (r *PostgresRepository) EditOwnMessage(ctx context.Context, messageID, accountID uuid.UUID, newBody string) (Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback(ctx)

	info, err := lockOwnedWebsiteMessage(ctx, tx, messageID, accountID)
	if err != nil {
		return Message{}, err
	}
	message, err := updateExistingMessage(ctx, tx, messageID, OperationEdit, newBody)
	if err != nil {
		return Message{}, err
	}
	if info.isDefault {
		if err := createOutboundTasksExcept(ctx, tx, messageID, "", OperationEdit, newBody); err != nil {
			return Message{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	message.ChannelKey = info.channelKey
	r.announce(ctx, info.channelKey, message)
	return message, nil
}

func (r *PostgresRepository) WithdrawOwnMessage(ctx context.Context, messageID, accountID uuid.UUID) (Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback(ctx)

	info, err := lockOwnedWebsiteMessage(ctx, tx, messageID, accountID)
	if err != nil {
		return Message{}, err
	}
	message, err := updateExistingMessage(ctx, tx, messageID, OperationDelete, "")
	if err != nil {
		return Message{}, err
	}
	if info.isDefault {
		if err := createOutboundTasksExcept(ctx, tx, messageID, "", OperationDelete, ""); err != nil {
			return Message{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	message.ChannelKey = info.channelKey
	r.announce(ctx, info.channelKey, message)
	return message, nil
}

func (r *PostgresRepository) SetReaction(ctx context.Context, messageID, accountID uuid.UUID, emoji string) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO chat_message_reactions (message_id, account_id, emoji)
		SELECT $1, $2, $3
		WHERE EXISTS (SELECT 1 FROM chat_messages WHERE id = $1 AND status <> 'deleted')
		ON CONFLICT DO NOTHING`, messageID, accountID, emoji)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Either the message is gone or the reaction already existed. Only the
		// former is an error the caller cares about.
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM chat_messages WHERE id = $1 AND status <> 'deleted')`, messageID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (r *PostgresRepository) RemoveReaction(ctx context.Context, messageID, accountID uuid.UUID, emoji string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM chat_message_reactions
		WHERE message_id = $1 AND account_id = $2 AND emoji = $3`, messageID, accountID, emoji)
	return err
}

func (r *PostgresRepository) SetPinned(ctx context.Context, messageID, adminID uuid.UUID, pinned bool) error {
	var tag pgconn.CommandTag
	var err error
	if pinned {
		tag, err = r.pool.Exec(ctx, `
			UPDATE chat_messages SET pinned_at = CURRENT_TIMESTAMP, pinned_by = $2
			WHERE id = $1 AND status <> 'deleted' AND pinned_at IS NULL`, messageID, adminID)
	} else {
		tag, err = r.pool.Exec(ctx, `
			UPDATE chat_messages SET pinned_at = NULL, pinned_by = NULL
			WHERE id = $1 AND pinned_at IS NOT NULL`, messageID)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM chat_messages WHERE id = $1 AND status <> 'deleted')`, messageID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		// Message exists but was already in the requested pin state: idempotent.
	}
	return nil
}

func collectMessages(rows pgx.Rows) ([]Message, error) {
	defer rows.Close()
	result := make([]Message, 0, 32)
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// enrich fills Reactions and ReplyCount for a page of messages in two aggregate
// queries keyed by message id.
func (r *PostgresRepository) enrich(ctx context.Context, msgs []Message, viewer uuid.UUID) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(msgs))
	index := make(map[uuid.UUID]int, len(msgs))
	for i := range msgs {
		ids[i] = msgs[i].ID
		index[msgs[i].ID] = i
	}

	reactionRows, err := r.pool.Query(ctx, `
		SELECT message_id, emoji, count(*)::int, bool_or(account_id = $2)
		FROM chat_message_reactions
		WHERE message_id = ANY($1)
		GROUP BY message_id, emoji
		ORDER BY count(*) DESC, emoji`, ids, viewer)
	if err != nil {
		return err
	}
	for reactionRows.Next() {
		var mid uuid.UUID
		var tally ReactionTally
		if err := reactionRows.Scan(&mid, &tally.Emoji, &tally.Count, &tally.Mine); err != nil {
			reactionRows.Close()
			return err
		}
		if i, ok := index[mid]; ok {
			msgs[i].Reactions = append(msgs[i].Reactions, tally)
		}
	}
	reactionRows.Close()
	if err := reactionRows.Err(); err != nil {
		return err
	}

	replyRows, err := r.pool.Query(ctx, `
		SELECT parent_message_id, count(*)::int
		FROM chat_messages
		WHERE parent_message_id = ANY($1) AND status <> 'deleted'
		GROUP BY parent_message_id`, ids)
	if err != nil {
		return err
	}
	for replyRows.Next() {
		var pid uuid.UUID
		var n int
		if err := replyRows.Scan(&pid, &n); err != nil {
			replyRows.Close()
			return err
		}
		if i, ok := index[pid]; ok {
			msgs[i].ReplyCount = n
		}
	}
	replyRows.Close()
	return replyRows.Err()
}
