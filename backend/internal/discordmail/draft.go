package discordmail

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// draftTTL is how long a 送出/編輯/刪除 preview stays actionable. Generous on
// purpose — a staff member reviewing a reply before sending shouldn't feel
// rushed — but bounded so an abandoned draft doesn't linger in Redis forever.
const draftTTL = 2 * time.Hour

// replyDraft is what a modal submission stashes before showing the staff
// member a preview — the 送出/編輯/刪除 buttons that follow only ever carry a
// short draft id (Discord's custom_id has a 100-character budget, nowhere
// near enough for a reply body), so the actual content has to live
// somewhere else in between.
type replyDraft struct {
	ThreadID       string `json:"thread_id"`
	StaffName      string `json:"staff_name"`
	StaffDiscordID string `json:"staff_discord_id"`
	Message        string `json:"message"`
	Signature      string `json:"signature"`
	// ApplicationID/Token identify the preview message this draft belongs
	// to (an ephemeral message is only addressable through the interaction
	// that created it, via .../webhooks/{application_id}/{token}/messages/
	// @original) — stored so a later, superseding draft can actually
	// delete it instead of leaving a dead-button message sitting in the
	// channel. Only usable within Discord's ~15 minute interaction-token
	// window; a supersede past that just can't clean up the old message,
	// no API call can.
	ApplicationID string `json:"application_id"`
	Token         string `json:"token"`
}

// DraftStore persists a replyDraft between the modal submission that creates
// it and whichever button (送出/編輯/刪除) the staff member eventually
// presses — those are separate Discord interactions with no shared state of
// their own, so this is the only place that connects them.
type DraftStore interface {
	// SaveDraft also supersedes whatever draft previously existed for
	// draft.ThreadID — only ever one draft per thread is live at a time, so
	// a fresh /re on a thread invalidates any earlier, not-yet-sent preview
	// for that same thread. The superseded draft (if any) is returned so
	// the caller can best-effort delete its now-stale preview message from
	// the channel.
	SaveDraft(ctx context.Context, id string, draft replyDraft) (superseded *replyDraft, err error)
	// UpdateDraft overwrites an existing draft's content in place — used by
	// 編輯, which revises a still-live draft rather than creating a new one.
	// Unlike SaveDraft it does not touch the thread->draft mapping or
	// supersede anything; id is assumed to already be that thread's current
	// draft.
	UpdateDraft(ctx context.Context, id string, draft replyDraft) error
	LoadDraft(ctx context.Context, id string) (replyDraft, bool, error)
	DeleteDraft(ctx context.Context, id string) error
}

// RedisDraftStore implements DraftStore. Drafts are small and short-lived
// enough that a plain string key with a TTL is simpler than anything
// fancier.
type RedisDraftStore struct {
	client *redis.Client
}

func NewRedisDraftStore(client *redis.Client) (*RedisDraftStore, error) {
	if client == nil {
		return nil, errors.New("redis client is required")
	}
	return &RedisDraftStore{client: client}, nil
}

func draftKey(id string) string {
	return "discordmail:reply-draft:" + id
}

// threadDraftKey tracks "which draft id is currently live for this thread" —
// what SaveDraft consults to supersede an older draft on the same thread.
func threadDraftKey(threadID string) string {
	return "discordmail:reply-draft-thread:" + threadID
}

func (s *RedisDraftStore) SaveDraft(ctx context.Context, id string, draft replyDraft) (*replyDraft, error) {
	var superseded *replyDraft
	if draft.ThreadID != "" {
		previousID, err := s.client.SetArgs(ctx, threadDraftKey(draft.ThreadID), id, redis.SetArgs{TTL: draftTTL, Get: true}).Result()
		if err == nil && previousID != "" && previousID != id {
			if previousDraft, found, loadErr := s.LoadDraft(ctx, previousID); loadErr == nil && found {
				superseded = &previousDraft
			}
			_ = s.client.Del(ctx, draftKey(previousID)).Err()
		}
	}
	encoded, err := json.Marshal(draft)
	if err != nil {
		return nil, err
	}
	if err := s.client.Set(ctx, draftKey(id), encoded, draftTTL).Err(); err != nil {
		return nil, err
	}
	return superseded, nil
}

func (s *RedisDraftStore) UpdateDraft(ctx context.Context, id string, draft replyDraft) error {
	encoded, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	// KEEPTTL rather than resetting draftTTL from scratch: an edit is a
	// continuation of the same draft's lifetime, not a new one.
	return s.client.Set(ctx, draftKey(id), encoded, redis.KeepTTL).Err()
}

func (s *RedisDraftStore) LoadDraft(ctx context.Context, id string) (replyDraft, bool, error) {
	raw, err := s.client.Get(ctx, draftKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return replyDraft{}, false, nil
	}
	if err != nil {
		return replyDraft{}, false, err
	}
	var draft replyDraft
	if err := json.Unmarshal(raw, &draft); err != nil {
		return replyDraft{}, false, err
	}
	return draft, true, nil
}

func (s *RedisDraftStore) DeleteDraft(ctx context.Context, id string) error {
	return s.client.Del(ctx, draftKey(id)).Err()
}

// newDraftID is an opaque, unguessable handle — nothing about a draft's
// content should be inferable from its id, since it rides along in a
// button's custom_id on an otherwise-ephemeral message.
func newDraftID() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
