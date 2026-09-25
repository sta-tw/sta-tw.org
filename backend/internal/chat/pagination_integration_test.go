//go:build integration

package chat

import (
	"context"
	"fmt"
	"testing"

	"sta-backend/internal/dbtest"
	"sta-backend/internal/pagination"
)

func TestListMessagesKeysetPagination(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	author := dbtest.InsertAccount(t, ctx, pool, "")

	const total = 25
	for i := 0; i < total; i++ {
		if _, err := repo.CreateWebsiteMessage(ctx, author, fmt.Sprintf("訊息 %02d", i)); err != nil {
			t.Fatalf("CreateWebsiteMessage %d: %v", i, err)
		}
	}

	seen := make(map[string]bool)
	var cursor pagination.Cursor
	pages := 0
	for {
		page, next, err := repo.ListMessages(ctx, 10, cursor)
		if err != nil {
			t.Fatalf("ListMessages page %d: %v", pages, err)
		}
		pages++
		for _, m := range page {
			if seen[m.ID.String()] {
				t.Fatalf("message %s returned on more than one page", m.ID)
			}
			seen[m.ID.String()] = true
		}
		if next == "" {
			if len(page) == 0 && pages > 1 {
				t.Fatalf("empty trailing page")
			}
			break
		}
		if len(page) != 10 {
			t.Fatalf("page %d has %d rows but a next cursor", pages, len(page))
		}
		cursor, err = pagination.Decode(next)
		if err != nil {
			t.Fatalf("Decode(next): %v", err)
		}
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
	}
	if len(seen) != total {
		t.Fatalf("saw %d distinct messages across %d pages, want %d", len(seen), pages, total)
	}
}
