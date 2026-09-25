//go:build integration

package schools

import (
	"context"
	"errors"
	"testing"

	"sta-backend/internal/dbtest"
)

func TestUpsertCreatesUpdatesAndAuditsSchools(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	admin := dbtest.InsertAdmin(t, ctx, pool)

	var code string
	if err := pool.QueryRow(ctx, `
		SELECT lpad(g::text, 3, '0') FROM generate_series(900, 999) g
		WHERE lpad(g::text, 3, '0') NOT IN (SELECT school_code FROM schools)
		LIMIT 1
	`).Scan(&code); err != nil {
		t.Fatalf("find free school code: %v", err)
	}

	active := true
	if _, err := repo.Upsert(ctx, admin, BatchInput{
		Reason: "integration create",
		Items:  []SchoolInput{{SchoolCode: code, SchoolName: "整合測試大學", InstitutionType: GeneralUniversity, IsActive: &active}},
	}); err != nil {
		t.Fatalf("Upsert create: %v", err)
	}

	if _, err := repo.Upsert(ctx, admin, BatchInput{
		Reason: "integration rename",
		Items:  []SchoolInput{{SchoolCode: code, SchoolName: "整合測試科技大學", InstitutionType: GeneralUniversity, IsActive: &active}},
	}); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	history, err := repo.ListHistory(ctx, admin, code)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("expected >=2 audit events (create + update), got %d", len(history))
	}

	// Non-admin is rejected before any write.
	plain := dbtest.InsertAccount(t, ctx, pool, "")
	if _, err := repo.Upsert(ctx, plain, BatchInput{
		Reason: "should fail",
		Items:  []SchoolInput{{SchoolCode: code, SchoolName: "x", InstitutionType: GeneralUniversity, IsActive: &active}},
	}); !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("non-admin Upsert error = %v, want ErrAdminRequired", err)
	}
}
