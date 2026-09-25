//go:build integration

package admin

import (
	"context"
	"fmt"
	"testing"

	"sta-backend/internal/dbtest"
)

func TestQueryAuditLogFilterAndKeyset(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	actor := dbtest.InsertAdmin(t, ctx, pool)

	for i := 0; i < 12; i++ {
		entity := "alpha"
		if i%2 == 0 {
			entity = "beta"
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
			VALUES ($1, 'x', $2, $3, '-')
		`, actor, entity, fmt.Sprintf("k%02d", i)); err != nil {
			t.Fatal(err)
		}
	}

	// Filter narrows to one entity_type.
	betas, err := queryAuditLog(ctx, pool, auditFilters{EntityType: "beta"}, 100, nil)
	if err != nil {
		t.Fatalf("queryAuditLog beta: %v", err)
	}
	if len(betas) != 6 {
		t.Fatalf("beta rows = %d, want 6", len(betas))
	}
	for _, e := range betas {
		if e.EntityType != "beta" {
			t.Fatalf("filter leaked entity_type %q", e.EntityType)
		}
	}

	// Keyset paging over the full set returns every row once, newest id first.
	seen := map[int64]bool{}
	var after *int64
	pages := 0
	for {
		page, err := queryAuditLog(ctx, pool, auditFilters{}, 5, after)
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		pages++
		for i, e := range page {
			if seen[e.ID] {
				t.Fatalf("row %d returned twice", e.ID)
			}
			seen[e.ID] = true
			if i > 0 && page[i-1].ID <= e.ID {
				t.Fatalf("page not ordered by id DESC: %d then %d", page[i-1].ID, e.ID)
			}
		}
		if len(page) < 5 {
			break
		}
		last := page[len(page)-1].ID
		after = &last
		if pages > 20 {
			t.Fatal("keyset paging did not terminate")
		}
	}
	if len(seen) < 12 {
		t.Fatalf("keyset walk saw %d rows, want >= 12", len(seen))
	}
}

func TestCollectStatsCountsSeededRows(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	dbtest.InsertAdmin(t, ctx, pool)
	dbtest.InsertStudent(t, ctx, pool)
	dbtest.InsertAccount(t, ctx, pool, "")

	before, err := collectStats(ctx, pool)
	if err != nil {
		t.Fatalf("collectStats: %v", err)
	}
	if before.Accounts.Total < 3 {
		t.Fatalf("accounts total = %d, want >= 3", before.Accounts.Total)
	}
	if before.Accounts.Students < 1 {
		t.Fatalf("accounts students = %d, want >= 1", before.Accounts.Students)
	}

	// A write to audit_log must move the counter.
	actor := dbtest.InsertAdmin(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
		VALUES ($1, 'test.action', 'test_entity', 'k1', 'integration test')
	`, actor); err != nil {
		t.Fatal(err)
	}
	after, err := collectStats(ctx, pool)
	if err != nil {
		t.Fatalf("collectStats after: %v", err)
	}
	if after.AuditLog.Total != before.AuditLog.Total+1 {
		t.Fatalf("audit_log total: before=%d after=%d, want +1", before.AuditLog.Total, after.AuditLog.Total)
	}

	// Every outbox health block is populated (zeroed, not errored).
	if after.Outbox.Email.Pending < 0 || after.Outbox.WillingnessNotifications.Abandoned < 0 {
		t.Fatal("outbox health not populated")
	}
}
