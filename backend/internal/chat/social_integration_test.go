//go:build integration

package chat

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"sta-backend/internal/dbtest"
	"sta-backend/internal/pagination"
)

func TestChannelsReactionsPinsThreads(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	author := dbtest.InsertAccount(t, ctx, pool, "")
	reactor := dbtest.InsertAccount(t, ctx, pool, "")
	admin := dbtest.InsertAdmin(t, ctx, pool)

	// A second, website-only channel alongside the seeded default 'lounge'.
	if _, err := pool.Exec(ctx, `
		INSERT INTO chat_channels (channel_key, display_name, kind) VALUES ('study', '讀書會', 'standard')`); err != nil {
		t.Fatal(err)
	}

	channels, err := repo.ListChannels(ctx)
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	var sawDefault, sawStudy bool
	for _, c := range channels {
		if c.Key == "lounge" && c.IsDefault {
			sawDefault = true
		}
		if c.Key == "study" {
			sawStudy = true
		}
	}
	if !sawDefault || !sawStudy {
		t.Fatalf("ListChannels missing entries: %+v", channels)
	}

	// Post to the non-default channel: no bridge outbox rows.
	root, err := repo.CreateChannelMessage(ctx, "study", author, "第一則", nil)
	if err != nil {
		t.Fatalf("CreateChannelMessage: %v", err)
	}
	if root.ChannelKey != "study" {
		t.Fatalf("channel key = %q", root.ChannelKey)
	}
	var outboxRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chat_sync_outbox WHERE message_id = $1`, root.ID).Scan(&outboxRows); err != nil {
		t.Fatal(err)
	}
	if outboxRows != 0 {
		t.Fatalf("non-default channel produced %d bridge tasks", outboxRows)
	}

	// A reply, then a reply to the reply must be rejected (one level deep).
	reply, err := repo.CreateChannelMessage(ctx, "study", reactor, "回覆", &root.ID)
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	if _, err := repo.CreateChannelMessage(ctx, "study", reactor, "巢狀", &reply.ID); err != ErrInvalidMessage {
		t.Fatalf("nested reply err = %v, want ErrInvalidMessage", err)
	}
	// A reply whose parent is in another channel is rejected.
	if _, err := repo.CreateChannelMessage(ctx, "lounge", reactor, "跨頻道", &root.ID); err != ErrInvalidMessage {
		t.Fatalf("cross-channel reply err = %v, want ErrInvalidMessage", err)
	}

	// Top-level listing excludes replies but carries a reply_count.
	top, _, err := repo.ListChannelMessages(ctx, "study", reactor, 50, pagination.Cursor{})
	if err != nil {
		t.Fatalf("ListChannelMessages: %v", err)
	}
	if len(top) != 1 || top[0].ID != root.ID || top[0].ReplyCount != 1 {
		t.Fatalf("top list = %+v (want 1 root with reply_count 1)", top)
	}

	replies, _, err := repo.ListThreadReplies(ctx, root.ID, reactor, 50, pagination.Cursor{})
	if err != nil || len(replies) != 1 || replies[0].ID != reply.ID {
		t.Fatalf("ListThreadReplies = %+v, %v", replies, err)
	}

	// Reactions: idempotent add, per-viewer "mine", removal.
	for i := 0; i < 2; i++ {
		if err := repo.SetReaction(ctx, root.ID, reactor, "👍"); err != nil {
			t.Fatalf("SetReaction %d: %v", i, err)
		}
	}
	if err := repo.SetReaction(ctx, root.ID, author, "👍"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetReaction(ctx, root.ID, author, "🎉"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetReaction(ctx, uuid.New(), reactor, "👍"); err != ErrNotFound {
		t.Fatalf("react to missing message err = %v, want ErrNotFound", err)
	}

	top, _, err = repo.ListChannelMessages(ctx, "study", reactor, 50, pagination.Cursor{})
	if err != nil {
		t.Fatal(err)
	}
	tallies := map[string]ReactionTally{}
	for _, tally := range top[0].Reactions {
		tallies[tally.Emoji] = tally
	}
	if tallies["👍"].Count != 2 || !tallies["👍"].Mine {
		t.Fatalf("👍 tally = %+v (want count 2, mine true)", tallies["👍"])
	}
	if tallies["🎉"].Count != 1 || tallies["🎉"].Mine {
		t.Fatalf("🎉 tally = %+v (want count 1, mine false for reactor)", tallies["🎉"])
	}

	if err := repo.RemoveReaction(ctx, root.ID, reactor, "👍"); err != nil {
		t.Fatal(err)
	}
	top, _, _ = repo.ListChannelMessages(ctx, "study", reactor, 50, pagination.Cursor{})
	for _, tally := range top[0].Reactions {
		if tally.Emoji == "👍" && (tally.Count != 1 || tally.Mine) {
			t.Fatalf("👍 after removal = %+v", tally)
		}
	}

	// Pins: set, idempotent, appears in the pins list, unset.
	if err := repo.SetPinned(ctx, root.ID, admin, true); err != nil {
		t.Fatalf("SetPinned true: %v", err)
	}
	if err := repo.SetPinned(ctx, root.ID, admin, true); err != nil {
		t.Fatalf("SetPinned true (idempotent): %v", err)
	}
	pins, err := repo.ListPinned(ctx, "study", reactor)
	if err != nil || len(pins) != 1 || pins[0].ID != root.ID || pins[0].PinnedAt == nil {
		t.Fatalf("ListPinned = %+v, %v", pins, err)
	}
	if err := repo.SetPinned(ctx, root.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if pins, _ := repo.ListPinned(ctx, "study", reactor); len(pins) != 0 {
		t.Fatalf("pin not cleared: %+v", pins)
	}
	if err := repo.SetPinned(ctx, uuid.New(), admin, true); err != ErrNotFound {
		t.Fatalf("pin missing message err = %v, want ErrNotFound", err)
	}

	// Unknown channel key surfaces as not-found.
	if _, _, err := repo.ListChannelMessages(ctx, "nope", reactor, 10, pagination.Cursor{}); err != ErrNotFound {
		t.Fatalf("unknown channel err = %v, want ErrNotFound", err)
	}
}

func TestEditWithdrawOwnMessage(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo, err := NewPostgresRepository(pool, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	author := dbtest.InsertAccount(t, ctx, pool, "")
	stranger := dbtest.InsertAccount(t, ctx, pool, "")

	msg, err := repo.CreateWebsiteMessage(ctx, author, "原始內容")
	if err != nil {
		t.Fatal(err)
	}

	// A stranger cannot edit or withdraw it.
	if _, err := repo.EditOwnMessage(ctx, msg.ID, stranger, "hijack"); err != ErrForbidden {
		t.Fatalf("stranger edit = %v, want ErrForbidden", err)
	}
	if _, err := repo.WithdrawOwnMessage(ctx, msg.ID, stranger); err != ErrForbidden {
		t.Fatalf("stranger withdraw = %v, want ErrForbidden", err)
	}

	// The author edits: body + status change, and the default channel queues a
	// bridge edit task.
	edited, err := repo.EditOwnMessage(ctx, msg.ID, author, "修改後內容")
	if err != nil {
		t.Fatalf("EditOwnMessage: %v", err)
	}
	if edited.Body != "修改後內容" || edited.Status != "edited" {
		t.Fatalf("edited = %+v", edited)
	}
	var editTasks int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chat_sync_outbox WHERE message_id = $1 AND operation = 'edit'`, msg.ID).Scan(&editTasks); err != nil {
		t.Fatal(err)
	}
	if editTasks == 0 {
		t.Fatal("no bridge edit task queued for a default-channel edit")
	}

	// The author withdraws: soft-deleted, dropped from listings, idempotent-ish
	// (a second attempt is now not-found).
	if _, err := repo.WithdrawOwnMessage(ctx, msg.ID, author); err != nil {
		t.Fatalf("WithdrawOwnMessage: %v", err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM chat_messages WHERE id = $1`, msg.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" {
		t.Fatalf("status after withdraw = %q", status)
	}
	if _, err := repo.EditOwnMessage(ctx, msg.ID, author, "too late"); err != ErrNotFound {
		t.Fatalf("edit after withdraw = %v, want ErrNotFound", err)
	}
	list, _, _ := repo.ListChannelMessages(ctx, "lounge", author, 50, pagination.Cursor{})
	for _, m := range list {
		if m.ID == msg.ID {
			t.Fatal("withdrawn message still listed")
		}
	}

	// A non-website (bridged) message cannot be edited via this path.
	ext := ExternalMessage{
		Platform: PlatformDiscord, ExternalMessageID: "d-1", ExternalAuthorID: "u-1",
		Body: "from discord", Operation: OperationCreate,
	}
	bridged, err := repo.ApplyExternalMessage(ctx, ext, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.EditOwnMessage(ctx, bridged.ID, author, "nope"); err != ErrForbidden {
		t.Fatalf("edit bridged message = %v, want ErrForbidden", err)
	}
}

func TestForwardMessage(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo, err := NewPostgresRepository(pool, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	author := dbtest.InsertAccount(t, ctx, pool, "")
	forwarder := dbtest.InsertAccount(t, ctx, pool, "")
	if _, err := pool.Exec(ctx, `INSERT INTO chat_channels (channel_key, display_name) VALUES ('archive', '存檔')`); err != nil {
		t.Fatal(err)
	}

	src, err := repo.CreateChannelMessage(ctx, "lounge", author, "值得留存的一句話", nil)
	if err != nil {
		t.Fatal(err)
	}

	fwd, err := repo.ForwardMessage(ctx, src.ID, "archive", forwarder)
	if err != nil {
		t.Fatalf("ForwardMessage: %v", err)
	}
	if fwd.Body != src.Body || fwd.ChannelKey != "archive" ||
		fwd.ForwardedFromID == nil || *fwd.ForwardedFromID != src.ID {
		t.Fatalf("forwarded message = %+v", fwd)
	}
	// It shows up in the target channel, still pointing at the source.
	list, _, err := repo.ListChannelMessages(ctx, "archive", forwarder, 10, pagination.Cursor{})
	if err != nil || len(list) != 1 || list[0].ForwardedFromID == nil || *list[0].ForwardedFromID != src.ID {
		t.Fatalf("archive list = %+v, %v", list, err)
	}
	// A non-default target queues no bridge tasks.
	var bridgeTasks int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chat_sync_outbox WHERE message_id = $1`, fwd.ID).Scan(&bridgeTasks); err != nil {
		t.Fatal(err)
	}
	if bridgeTasks != 0 {
		t.Fatalf("non-default forward queued %d bridge tasks", bridgeTasks)
	}

	// Forwarding a missing / deleted source or to an unknown channel fails.
	if _, err := repo.ForwardMessage(ctx, uuid.New(), "archive", forwarder); err != ErrNotFound {
		t.Fatalf("forward missing source = %v, want ErrNotFound", err)
	}
	if _, err := repo.ForwardMessage(ctx, src.ID, "ghost", forwarder); err != ErrNotFound {
		t.Fatalf("forward to unknown channel = %v, want ErrNotFound", err)
	}
}
