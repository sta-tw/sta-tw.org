package mailroutes

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// localPartPattern mirrors what Postfix's pgsql map + RFC 5321 local-parts
// can safely handle without quoting games: lowercase ascii, digits, and a
// couple of separators. Kept intentionally narrower than what email
// actually allows.
var localPartPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

var ErrInvalidInput = errors.New("invalid input")
var ErrDuplicateLocalPart = errors.New("local part already exists")
var ErrNotFound = errors.New("mail route not found")

// validateReplyTemplate mirrors admin.setMailReplyTemplate's rule (must
// contain both [內容] and [簽名]) for a route-level override. nil or
// blank clears the override back to the global default.
func validateReplyTemplate(template *string) (*string, error) {
	if template == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*template)
	if trimmed == "" {
		return nil, nil
	}
	if !strings.Contains(trimmed, "[內容]") || !strings.Contains(trimmed, "[簽名]") {
		return nil, ErrInvalidInput
	}
	return &trimmed, nil
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`,
		accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) List(ctx context.Context) ([]MailRoute, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, local_part, label, discord_forum_channel_id, visible_role_ids, reply_template, created_at
		FROM mail_routes
		ORDER BY local_part
	`)
	if err != nil {
		return nil, fmt.Errorf("list mail routes: %w", err)
	}
	defer rows.Close()
	items := make([]MailRoute, 0)
	for rows.Next() {
		var m MailRoute
		if err := rows.Scan(&m.ID, &m.LocalPart, &m.Label, &m.DiscordForumChannelID, &m.VisibleRoleIDs, &m.ReplyTemplate, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mail route: %w", err)
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) Create(ctx context.Context, createdBy uuid.UUID, input MailRouteInput, channelID string) (MailRoute, error) {
	localPart := strings.ToLower(strings.TrimSpace(input.LocalPart))
	label := strings.TrimSpace(input.Label)
	channelID = strings.TrimSpace(channelID)
	if !localPartPattern.MatchString(localPart) || label == "" || channelID == "" {
		return MailRoute{}, ErrInvalidInput
	}
	replyTemplate, err := validateReplyTemplate(input.ReplyTemplate)
	if err != nil {
		return MailRoute{}, err
	}
	var m MailRoute
	err = r.pool.QueryRow(ctx, `
		INSERT INTO mail_routes (local_part, label, discord_forum_channel_id, visible_role_ids, reply_template, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, local_part, label, discord_forum_channel_id, visible_role_ids, reply_template, created_at
	`, localPart, label, channelID, input.VisibleRoleIDs, replyTemplate, createdBy).Scan(&m.ID, &m.LocalPart, &m.Label, &m.DiscordForumChannelID, &m.VisibleRoleIDs, &m.ReplyTemplate, &m.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return MailRoute{}, ErrDuplicateLocalPart
		}
		return MailRoute{}, fmt.Errorf("create mail route: %w", err)
	}
	return m, nil
}

func (r *PostgresRepository) GetByLocalPart(ctx context.Context, localPart string) (MailRoute, error) {
	var m MailRoute
	err := r.pool.QueryRow(ctx, `
		SELECT id, local_part, label, discord_forum_channel_id, visible_role_ids, reply_template, created_at
		FROM mail_routes WHERE local_part = $1
	`, strings.ToLower(strings.TrimSpace(localPart))).Scan(&m.ID, &m.LocalPart, &m.Label, &m.DiscordForumChannelID, &m.VisibleRoleIDs, &m.ReplyTemplate, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MailRoute{}, ErrNotFound
	}
	if err != nil {
		return MailRoute{}, fmt.Errorf("get mail route by local part: %w", err)
	}
	return m, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (MailRoute, error) {
	var m MailRoute
	err := r.pool.QueryRow(ctx, `
		SELECT id, local_part, label, discord_forum_channel_id, visible_role_ids, reply_template, created_at
		FROM mail_routes WHERE id = $1
	`, id).Scan(&m.ID, &m.LocalPart, &m.Label, &m.DiscordForumChannelID, &m.VisibleRoleIDs, &m.ReplyTemplate, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MailRoute{}, ErrNotFound
	}
	if err != nil {
		return MailRoute{}, fmt.Errorf("get mail route: %w", err)
	}
	return m, nil
}

func (r *PostgresRepository) VisibleRoleIDs(ctx context.Context, id uuid.UUID) ([]string, error) {
	var roleIDs []string
	err := r.pool.QueryRow(ctx, `SELECT visible_role_ids FROM mail_routes WHERE id = $1`, id).Scan(&roleIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get mail route visible role ids: %w", err)
	}
	return roleIDs, nil
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, label string, visibleRoleIDs []string, replyTemplateInput *string) (MailRoute, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return MailRoute{}, ErrInvalidInput
	}
	replyTemplate, err := validateReplyTemplate(replyTemplateInput)
	if err != nil {
		return MailRoute{}, err
	}
	var m MailRoute
	err = r.pool.QueryRow(ctx, `
		UPDATE mail_routes SET label = $2, visible_role_ids = $3, reply_template = $4
		WHERE id = $1
		RETURNING id, local_part, label, discord_forum_channel_id, visible_role_ids, reply_template, created_at
	`, id, label, visibleRoleIDs, replyTemplate).Scan(&m.ID, &m.LocalPart, &m.Label, &m.DiscordForumChannelID, &m.VisibleRoleIDs, &m.ReplyTemplate, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MailRoute{}, ErrNotFound
	}
	if err != nil {
		return MailRoute{}, fmt.Errorf("update mail route: %w", err)
	}
	return m, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM mail_routes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete mail route: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
