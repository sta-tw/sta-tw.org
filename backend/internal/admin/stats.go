package admin

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StatsSnapshot is the payload of GET /api/v1/admin/stats: a point-in-time
// count of the platform's main entities and the health of every retry outbox.
type StatsSnapshot struct {
	GeneratedAt time.Time `json:"generated_at"`

	Accounts struct {
		Total     int64 `json:"total"`
		Active    int64 `json:"active"`
		Suspended int64 `json:"suspended"`
		Deleted   int64 `json:"deleted"`
		Students  int64 `json:"students"`
		Seniors   int64 `json:"seniors"`
		Verified  int64 `json:"verified"`
	} `json:"accounts"`

	Applications struct {
		Total     int64 `json:"total"`
		Draft     int64 `json:"draft"`
		Confirmed int64 `json:"confirmed"`
		Withdrawn int64 `json:"withdrawn"`
		Archived  int64 `json:"archived"`
	} `json:"applications"`

	Experiences struct {
		Total       int64 `json:"total"`
		Published   int64 `json:"published"`
		Hidden      int64 `json:"hidden"`
		Unpublished int64 `json:"unpublished"`
	} `json:"experiences"`

	Forum struct {
		Spaces  int64 `json:"spaces"`
		Threads int64 `json:"threads"`
		Posts   int64 `json:"posts"`
	} `json:"forum"`

	Chat struct {
		LoungeMessages int64 `json:"lounge_messages"`
	} `json:"chat"`

	SupportTickets struct {
		Total  int64 `json:"total"`
		Open   int64 `json:"open"`
		Closed int64 `json:"closed"`
	} `json:"support_tickets"`

	VerificationRequests struct {
		Pending  int64 `json:"pending"`
		Approved int64 `json:"approved"`
		Rejected int64 `json:"rejected"`
	} `json:"verification_requests"`

	ResultBatches struct {
		Total         int64 `json:"total"`
		PendingReview int64 `json:"pending_review"`
		Published     int64 `json:"published"`
	} `json:"result_batches"`

	AuditLog struct {
		Total int64 `json:"total"`
	} `json:"audit_log"`

	Outbox struct {
		Email                    OutboxHealth `json:"email"`
		ChatSync                 OutboxHealth `json:"chat_sync"`
		SupportDiscord           OutboxHealth `json:"support_discord"`
		WillingnessNotifications OutboxHealth `json:"willingness_notifications"`
	} `json:"outbox"`
}

// OutboxHealth is the backlog of one retry queue. abandoned > 0 means messages
// hit max_attempts and were dropped — the signal an operator alerts on.
type OutboxHealth struct {
	Pending   int64 `json:"pending"`
	Failed    int64 `json:"failed"`
	Abandoned int64 `json:"abandoned"`
}

func collectStats(ctx context.Context, pool *pgxpool.Pool) (*StatsSnapshot, error) {
	var s StatsSnapshot
	s.GeneratedAt = time.Now().UTC()

	if err := pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE account_status = 'active'),
		       count(*) FILTER (WHERE account_status = 'suspended'),
		       count(*) FILTER (WHERE account_status = 'deleted'),
		       count(*) FILTER (WHERE identity_status = 'student'),
		       count(*) FILTER (WHERE identity_status = 'senior'),
		       count(*) FILTER (WHERE email_verified_at IS NOT NULL)
		FROM accounts
	`).Scan(&s.Accounts.Total, &s.Accounts.Active, &s.Accounts.Suspended, &s.Accounts.Deleted,
		&s.Accounts.Students, &s.Accounts.Seniors, &s.Accounts.Verified); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'draft'),
		       count(*) FILTER (WHERE status = 'confirmed'),
		       count(*) FILTER (WHERE status = 'withdrawn'),
		       count(*) FILTER (WHERE status = 'archived')
		FROM applications
	`).Scan(&s.Applications.Total, &s.Applications.Draft, &s.Applications.Confirmed,
		&s.Applications.Withdrawn, &s.Applications.Archived); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE visibility = 'published'),
		       count(*) FILTER (WHERE visibility = 'hidden'),
		       count(*) FILTER (WHERE visibility = 'unpublished')
		FROM experiences
	`).Scan(&s.Experiences.Total, &s.Experiences.Published, &s.Experiences.Hidden,
		&s.Experiences.Unpublished); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM forum_spaces),
		       (SELECT count(*) FROM forum_threads WHERE status = 'published'),
		       (SELECT count(*) FROM forum_posts   WHERE status = 'published')
	`).Scan(&s.Forum.Spaces, &s.Forum.Threads, &s.Forum.Posts); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM chat_messages WHERE status <> 'deleted'
	`).Scan(&s.Chat.LoungeMessages); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE closed_at IS NULL),
		       count(*) FILTER (WHERE closed_at IS NOT NULL)
		FROM support_tickets
	`).Scan(&s.SupportTickets.Total, &s.SupportTickets.Open, &s.SupportTickets.Closed); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'approved'),
		       count(*) FILTER (WHERE status = 'rejected')
		FROM verification_requests
	`).Scan(&s.VerificationRequests.Pending, &s.VerificationRequests.Approved,
		&s.VerificationRequests.Rejected); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'pending_review'),
		       count(*) FILTER (WHERE status = 'published')
		FROM official_result_batches
	`).Scan(&s.ResultBatches.Total, &s.ResultBatches.PendingReview,
		&s.ResultBatches.Published); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&s.AuditLog.Total); err != nil {
		return nil, err
	}

	for _, q := range []struct {
		table, statusCol string
		dst              *OutboxHealth
	}{
		{"email_outbox", "status", &s.Outbox.Email},
		{"chat_sync_outbox", "status", &s.Outbox.ChatSync},
		{"support_discord_outbox", "status", &s.Outbox.SupportDiscord},
		{"willingness_inquiries", "notification_status", &s.Outbox.WillingnessNotifications},
	} {
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE `+q.statusCol+` = 'pending'),
			       count(*) FILTER (WHERE `+q.statusCol+` = 'failed'),
			       count(*) FILTER (WHERE `+q.statusCol+` = 'abandoned')
			FROM `+q.table,
		).Scan(&q.dst.Pending, &q.dst.Failed, &q.dst.Abandoned); err != nil {
			return nil, err
		}
	}

	return &s, nil
}
