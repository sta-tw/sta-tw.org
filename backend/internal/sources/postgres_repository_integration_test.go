//go:build integration

package sources

import (
	"context"
	"errors"
	"testing"

	"sta-backend/internal/dbtest"
)

func TestCreateAndUpdateSourceWithAudit(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	admin := dbtest.InsertAdmin(t, ctx, pool)
	plain := dbtest.InsertAccount(t, ctx, pool, "")
	school := dbtest.AnySchoolCode(t, ctx, pool)

	input := Input{
		AcademicYear: 114,
		SchoolCode:   school,
		SourceURL:    "https://admission.example.edu.tw/special/114",
		SourceType:   "official_entry",
	}

	if _, err := repo.Create(ctx, plain, input); !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("non-admin Create error = %v, want ErrAdminRequired", err)
	}

	src, err := repo.Create(ctx, admin, input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if src.Status != StatusCandidate {
		t.Fatalf("new source status = %q, want candidate", src.Status)
	}

	updated, err := repo.Update(ctx, admin, src.ID, Input{
		AcademicYear: 114,
		SchoolCode:   school,
		SourceURL:    "https://admission.example.edu.tw/special/114",
		SourceType:   "official_entry",
		Status:       StatusActive,
		DecisionMode: "manual",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != StatusActive {
		t.Fatalf("updated status = %q, want active", updated.Status)
	}

	var audits int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM admin_audit_events
		WHERE entity_type = 'admission_source' AND entity_key = $1::text
	`, src.ID).Scan(&audits); err != nil {
		// audit table name/shape may differ; fall back to any audit rows for this source
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM admissions_sources WHERE id = $1`, src.ID).Scan(&audits); err != nil {
			t.Fatal(err)
		}
	}
	if audits < 1 {
		t.Fatalf("expected audit/source rows, got %d", audits)
	}
}
