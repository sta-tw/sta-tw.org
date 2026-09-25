//go:build integration

package content

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"sta-backend/internal/dbtest"
	"sta-backend/internal/pagination"
)

func TestContentReactions(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	author := dbtest.InsertAccount(t, ctx, pool, "")
	reactor := dbtest.InsertAccount(t, ctx, pool, "")

	var spaceID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM forum_spaces WHERE space_type = 'global' LIMIT 1`).Scan(&spaceID); err != nil {
		t.Fatal(err)
	}
	var threadID, postID, expID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO forum_threads (space_id, account_id, title) VALUES ($1, $2, 't') RETURNING id`, spaceID, author).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO forum_posts (thread_id, account_id, body) VALUES ($1, $2, 'hi') RETURNING id`, threadID, author).Scan(&postID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO experiences (author_account_id, visibility) VALUES ($1, 'published') RETURNING id`, author).Scan(&expID); err != nil {
		t.Fatal(err)
	}
	var revID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO experience_revisions (experience_id, version_number, title, body, review_status, reviewed_at)
		VALUES ($1, 1, 'title', 'body', 'approved', CURRENT_TIMESTAMP) RETURNING id`, expID).Scan(&revID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE experiences SET current_public_revision_id = $2 WHERE id = $1`, expID, revID); err != nil {
		t.Fatal(err)
	}

	// Add reactions to both target types; adding twice is idempotent.
	for i := 0; i < 2; i++ {
		if err := repo.SetReaction(ctx, ReactionTargetPost, postID, reactor, "👍"); err != nil {
			t.Fatalf("SetReaction post %d: %v", i, err)
		}
	}
	if err := repo.SetReaction(ctx, ReactionTargetPost, postID, author, "👍"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetReaction(ctx, ReactionTargetExperience, expID, reactor, "🎉"); err != nil {
		t.Fatal(err)
	}

	// Unknown target type and missing target id both error.
	if err := repo.SetReaction(ctx, "bogus", postID, reactor, "👍"); err != ErrInvalidReaction {
		t.Fatalf("bogus target type = %v, want ErrInvalidReaction", err)
	}
	if err := repo.SetReaction(ctx, ReactionTargetPost, uuid.New(), reactor, "👍"); err != ErrNotFound {
		t.Fatalf("missing post = %v, want ErrNotFound", err)
	}

	// loadReactions tallies with a per-viewer "mine".
	byPost, err := repo.loadReactions(ctx, ReactionTargetPost, []uuid.UUID{postID}, reactor)
	if err != nil {
		t.Fatal(err)
	}
	if len(byPost[postID]) != 1 || byPost[postID][0].Count != 2 || !byPost[postID][0].Mine {
		t.Fatalf("post tally = %+v", byPost[postID])
	}
	byPostOther, _ := repo.loadReactions(ctx, ReactionTargetPost, []uuid.UUID{postID}, uuid.Nil)
	if byPostOther[postID][0].Mine {
		t.Fatal("anonymous viewer should not own a reaction")
	}

	// Enrichment reaches the list/get read paths.
	posts, _, err := repo.ListPosts(ctx, &reactor, threadID, 10, pagination.Cursor{})
	if err != nil {
		t.Fatalf("ListPosts: %v", err)
	}
	if len(posts) != 1 || len(posts[0].Reactions) != 1 || posts[0].Reactions[0].Count != 2 {
		t.Fatalf("ListPosts reactions = %+v", posts)
	}
	exp, err := repo.GetExperience(ctx, &reactor, expID)
	if err != nil {
		t.Fatalf("GetExperience: %v", err)
	}
	if len(exp.Reactions) != 1 || exp.Reactions[0].Emoji != "🎉" {
		t.Fatalf("experience reactions = %+v", exp.Reactions)
	}

	// Remove drops the tally.
	if err := repo.RemoveReaction(ctx, ReactionTargetPost, postID, reactor, "👍"); err != nil {
		t.Fatal(err)
	}
	byPost, _ = repo.loadReactions(ctx, ReactionTargetPost, []uuid.UUID{postID}, reactor)
	if byPost[postID][0].Count != 1 || byPost[postID][0].Mine {
		t.Fatalf("post tally after remove = %+v", byPost[postID])
	}
}
