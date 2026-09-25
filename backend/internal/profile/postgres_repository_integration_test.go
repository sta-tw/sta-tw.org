//go:build integration

package profile

import (
	"context"
	"testing"

	"sta-backend/internal/dbtest"
)

func TestProfileRepositoryLifecycle(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}

	acct := dbtest.InsertAccount(t, ctx, pool, "profile-user")
	other := dbtest.InsertAccount(t, ctx, pool, "")

	// No row yet: Get returns an empty-but-valid profile carrying account fields.
	p, err := repo.Get(ctx, acct)
	if err != nil {
		t.Fatalf("Get(no row): %v", err)
	}
	if p.Username != "profile-user" || p.DisplayName != "" || p.Links == nil || len(p.Links) != 0 || p.HasAvatar {
		t.Fatalf("empty profile = %+v", p)
	}

	// Upsert twice: create then update, links round-trip through jsonb.
	in := Input{DisplayName: "  Nova  ", Bio: "hi", Links: []Link{{Label: "site", URL: "https://example.test/x"}}}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Upsert(ctx, acct, in); err != nil {
		t.Fatalf("Upsert create: %v", err)
	}
	in.DisplayName = "Nova2"
	in.Links = nil
	_ = in.Normalize()
	p, err = repo.Upsert(ctx, acct, in)
	if err != nil {
		t.Fatalf("Upsert update: %v", err)
	}
	if p.DisplayName != "Nova2" || len(p.Links) != 0 {
		t.Fatalf("updated profile = %+v", p)
	}

	// GetByUsername is case-insensitive.
	byName, err := repo.GetByUsername(ctx, "PROFILE-USER")
	if err != nil || byName.AccountID != acct {
		t.Fatalf("GetByUsername = %+v, %v", byName, err)
	}
	if _, err := repo.GetByUsername(ctx, "nobody"); err != ErrNotFound {
		t.Fatalf("GetByUsername(missing) = %v, want ErrNotFound", err)
	}

	// Avatar: first set returns no old key; replacing returns the previous one.
	old, err := repo.SetAvatar(ctx, acct, "avatars/a/1", "image/png")
	if err != nil || old != "" {
		t.Fatalf("SetAvatar first = %q, %v", old, err)
	}
	old, err = repo.SetAvatar(ctx, acct, "avatars/a/2", "image/png")
	if err != nil || old != "avatars/a/1" {
		t.Fatalf("SetAvatar replace = %q, %v; want avatars/a/1", old, err)
	}
	if key, err := repo.AvatarByAccountID(ctx, acct); err != nil || key != "avatars/a/2" {
		t.Fatalf("AvatarByAccountID = %q, %v", key, err)
	}
	if key, err := repo.AvatarByUsername(ctx, "profile-user"); err != nil || key != "avatars/a/2" {
		t.Fatalf("AvatarByUsername = %q, %v", key, err)
	}

	old, err = repo.ClearAvatar(ctx, acct)
	if err != nil || old != "avatars/a/2" {
		t.Fatalf("ClearAvatar = %q, %v; want avatars/a/2", old, err)
	}
	if _, err := repo.AvatarByAccountID(ctx, acct); err != ErrNotFound {
		t.Fatalf("AvatarByAccountID after clear = %v, want ErrNotFound", err)
	}
	// Clearing again (row exists, no avatar) is a no-op with no old key.
	if old, err := repo.ClearAvatar(ctx, acct); err != nil || old != "" {
		t.Fatalf("ClearAvatar idempotent = %q, %v", old, err)
	}
	// Account with no profile row at all.
	if _, err := repo.ClearAvatar(ctx, other); err != nil {
		t.Fatalf("ClearAvatar(no row) = %v", err)
	}
	if _, err := repo.AvatarByAccountID(ctx, other); err != ErrNotFound {
		t.Fatalf("AvatarByAccountID(no row) = %v, want ErrNotFound", err)
	}
}
