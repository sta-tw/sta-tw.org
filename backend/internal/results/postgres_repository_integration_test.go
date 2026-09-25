//go:build integration

package results

import (
	"context"
	"errors"
	"testing"

	"sta-backend/internal/dbtest"
)

func TestImportAndPublishOfficialBatch(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin := dbtest.InsertAdmin(t, ctx, pool)
	plain := dbtest.InsertAccount(t, ctx, pool, "")
	school := dbtest.AnySchoolCode(t, ctx, pool)
	dbtest.InsertPublishedProgram(t, ctx, pool, 114, school, "901")

	rank1 := 1
	input := ImportBatchInput{
		AcademicYear: 114,
		SchoolCode:   school,
		SourceURL:    "https://admission.example.edu.tw/114-results.pdf",
		SourceSHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		Rows: []OfficialResultRow{
			{AcademicYear: 114, SchoolCode: school, ProgramCode: "901", CandidateNumber: "A1234567", MaskedName: "王〇〇", ResultStatus: ResultStatusAdmitted, OfficialRank: &rank1, SourcePage: 1},
			{AcademicYear: 114, SchoolCode: school, ProgramCode: "901", CandidateNumber: "B7654321", MaskedName: "李〇〇", ResultStatus: ResultStatusRejected, SourcePage: 1},
		},
	}

	if _, err := repo.ImportOfficialBatch(ctx, plain, input); !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("non-admin import error = %v, want ErrAdminRequired", err)
	}

	batchID, err := repo.ImportOfficialBatch(ctx, admin, input)
	if err != nil {
		t.Fatalf("ImportOfficialBatch: %v", err)
	}

	var status string
	var rowCount int
	if err := pool.QueryRow(ctx, `SELECT status FROM official_result_batches WHERE id=$1`, batchID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != ResultBatchStatusPendingReview {
		t.Fatalf("imported batch status = %q, want pending_review", status)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM official_results WHERE batch_id=$1`, batchID).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 2 {
		t.Fatalf("imported %d result rows, want 2", rowCount)
	}

	if err := repo.PublishOfficialBatch(ctx, admin, batchID); err != nil {
		t.Fatalf("PublishOfficialBatch: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM official_result_batches WHERE id=$1`, batchID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != ResultBatchStatusPublished {
		t.Fatalf("published batch status = %q", status)
	}

	// Re-publishing a batch that is no longer pending_review is rejected.
	if err := repo.PublishOfficialBatch(ctx, admin, batchID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second publish error = %v, want ErrNotFound", err)
	}
}
