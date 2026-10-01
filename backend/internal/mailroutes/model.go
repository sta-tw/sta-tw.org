// Package mailroutes is the admin-managed list of inbound mail categories:
// each row is one local-part at the existing mail domain (e.g. "brochure"
// for brochure@mail.sta-tw.org) wired to a Discord forum channel. Postfix
// looks this table up directly (pgsql maps), so creating a route here makes
// it live immediately with no deploy — see backend/deploy/postfix/.
package mailroutes

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type MailRoute struct {
	ID                    uuid.UUID `json:"id"`
	LocalPart             string    `json:"local_part"`
	Label                 string    `json:"label"`
	DiscordForumChannelID string    `json:"discord_forum_channel_id"`
	// VisibleRoleIDs is empty until an admin picks at least one Discord
	// role for this route — see PermissionsClient.ApplyChannelVisibility.
	VisibleRoleIDs []string `json:"visible_role_ids"`
	// ReplyTemplate overrides the global app_settings.mail_reply_template
	// for /re replies sent from this route's threads — nil means "use the
	// global default" (see emailinquiries.Repository.ReplyTemplate).
	ReplyTemplate *string   `json:"reply_template"`
	CreatedAt     time.Time `json:"created_at"`
}

// MailRouteInput no longer takes a channel id from the admin: the API
// creates the forum channel itself (parented under
// config.DiscordMailForumCategoryID) and fills DiscordForumChannelID in on
// the returned MailRoute.
type MailRouteInput struct {
	LocalPart      string   `json:"local_part"`
	Label          string   `json:"label"`
	VisibleRoleIDs []string `json:"visible_role_ids"`
	// ReplyTemplate is optional; nil or blank clears the override back to
	// the global default. A non-blank value must contain [內容] and [簽名]
	// — see PostgresRepository's validateReplyTemplate.
	ReplyTemplate *string `json:"reply_template"`
}

// DiscordRole is one entry from GET /guilds/{guild_id}/roles, offered in the
// admin UI's role picker.
type DiscordRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PermissionsClient talks to Discord to create a route's forum channel, keep
// its visibility in sync with VisibleRoleIDs, and list a guild's roles for
// the admin UI's picker.
type PermissionsClient interface {
	ListGuildRoles(ctx context.Context) ([]DiscordRole, error)
	// CreateForumChannel creates a new Forum channel named after name,
	// parented under the configured category (see
	// config.DiscordMailForumCategoryID) — this is the channel a new mail
	// route's inquiries get posted into.
	CreateForumChannel(ctx context.Context, name string) (channelID string, err error)
	// ApplyChannelVisibility denies @everyone and allows exactly roleIDs to
	// view channelID. A no-op when roleIDs is empty (channel permissions are
	// left untouched — see the visible_role_ids column comment).
	ApplyChannelVisibility(ctx context.Context, channelID string, roleIDs []string) error
	// LockChannel is what a deleted mail route falls back to instead of
	// actually deleting the Discord channel (its post history is worth
	// keeping) — denies @everyone and removes every previously-granted
	// role's ALLOW overwrite, so nobody can see it anymore regardless of
	// role (a role-level ALLOW would otherwise still beat @everyone's DENY).
	LockChannel(ctx context.Context, channelID string, previouslyVisibleRoleIDs []string) error
	// ArchiveAllThreads archives (and locks) every currently-active forum
	// post in channelID — the other half of decommissioning a mail route's
	// channel, done alongside LockChannel.
	ArchiveAllThreads(ctx context.Context, channelID string) error
}

type Repository interface {
	IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error)
	List(ctx context.Context) ([]MailRoute, error)
	// Create's channelID is the Discord forum channel PermissionsClient
	// already created for this route — the repository never talks to
	// Discord itself.
	Create(ctx context.Context, createdBy uuid.UUID, input MailRouteInput, channelID string) (MailRoute, error)
	// Update changes label/visible_role_ids only — local_part and the
	// Discord channel itself are not editable here (changing local_part
	// would silently break the address people already have; the channel is
	// managed by ApplyChannelVisibility, not by hand).
	Update(ctx context.Context, id uuid.UUID, label string, visibleRoleIDs []string, replyTemplate *string) (MailRoute, error)
	Get(ctx context.Context, id uuid.UUID) (MailRoute, error)
	// VisibleRoleIDs is the lighter-weight half of Get — used by
	// emailinquiries.Service to know which roles to @-mention when mail
	// arrives for this route, without needing the rest of the row.
	VisibleRoleIDs(ctx context.Context, id uuid.UUID) ([]string, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// GetByLocalPart resolves an inbound recipient's local-part (e.g.
	// "brochure" for brochure@mail.sta-tw.org) to its route — used by
	// accountmail.Handler to know which Discord forum channel a brand new
	// email belongs to.
	GetByLocalPart(ctx context.Context, localPart string) (MailRoute, error)
}
