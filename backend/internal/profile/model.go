// Package profile serves opt-in public account profiles and their avatars.
// A profile row is created on first edit; the avatar is stored privately in
// object storage and reached through a short-lived presigned redirect.
package profile

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("profile not found")
	ErrInvalid  = errors.New("profile is invalid")
)

const (
	maxDisplayName = 80
	maxBio         = 500
	maxLinks       = 10
	maxLinkLabel   = 40
	maxLinkURL     = 300
)

// Link is one labelled external URL on a profile.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Profile is the public view returned by GET /api/v1/users/{username} and the
// self view from GET /api/v1/profile.
type Profile struct {
	AccountID       uuid.UUID  `json:"account_id"`
	Username        string     `json:"username"`
	IdentityStatus  string     `json:"identity_status"`
	DisplayName     string     `json:"display_name,omitempty"`
	Bio             string     `json:"bio,omitempty"`
	Links           []Link     `json:"links"`
	HasAvatar       bool       `json:"has_avatar"`
	AvatarUpdatedAt *time.Time `json:"avatar_updated_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// Input is the editable part of a profile.
type Input struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	Links       []Link `json:"links"`
}

// Normalize trims fields and validates lengths, link count and URL schemes.
func (in *Input) Normalize() error {
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Bio = strings.TrimSpace(in.Bio)
	if len([]rune(in.DisplayName)) > maxDisplayName || len([]rune(in.Bio)) > maxBio {
		return ErrInvalid
	}
	if len(in.Links) > maxLinks {
		return ErrInvalid
	}
	cleaned := make([]Link, 0, len(in.Links))
	for _, link := range in.Links {
		label := strings.TrimSpace(link.Label)
		raw := strings.TrimSpace(link.URL)
		if raw == "" {
			continue
		}
		if len([]rune(label)) > maxLinkLabel || len(raw) > maxLinkURL {
			return ErrInvalid
		}
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return ErrInvalid
		}
		if label == "" {
			label = parsed.Host
		}
		cleaned = append(cleaned, Link{Label: label, URL: raw})
	}
	in.Links = cleaned
	return nil
}

// Repository is the persistence contract for profiles.
type Repository interface {
	Get(ctx context.Context, accountID uuid.UUID) (Profile, error)
	GetByUsername(ctx context.Context, username string) (Profile, error)
	Upsert(ctx context.Context, accountID uuid.UUID, in Input) (Profile, error)
	SetAvatar(ctx context.Context, accountID uuid.UUID, storageKey, contentType string) (oldKey string, err error)
	ClearAvatar(ctx context.Context, accountID uuid.UUID) (oldKey string, err error)
	AvatarByUsername(ctx context.Context, username string) (storageKey string, err error)
	AvatarByAccountID(ctx context.Context, accountID uuid.UUID) (storageKey string, err error)
}
