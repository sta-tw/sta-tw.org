//go:build integration

package admissions

import (
	"context"
	"errors"
	"testing"

	"sta-backend/internal/dbtest"
)

func TestUpsertThenReviewProgramPublishes(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	admin := dbtest.InsertAdmin(t, ctx, pool)
	plain := dbtest.InsertAccount(t, ctx, pool, "")
	school := dbtest.AnySchoolCode(t, ctx, pool)

	id := ProgramIdentifier{AcademicYear: 114, SchoolCode: school, ProgramCode: "710"}
	weight := 100.0
	batch := ProgramBatchInput{
		Reason: "integration import",
		Items: []ProgramInput{{
			AcademicYear:         114,
			SchoolCode:           school,
			ProgramCode:          "710",
			AdmissionProgramName: "整合測試學系",
			AdmissionQuota:       3,
			ExamItems:            []ExamItem{{Name: "資料審查", SortOrder: 1, WeightPercent: &weight}},
		}},
	}

	if _, err := repo.UpsertPrograms(ctx, plain, batch); !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("non-admin upsert error = %v, want ErrAdminRequired", err)
	}
	if _, err := repo.UpsertPrograms(ctx, admin, batch); err != nil {
		t.Fatalf("UpsertPrograms: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT review_status FROM academic_programs WHERE academic_year=114 AND school_code=$1 AND program_code='710'`, school).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != ProgramStatusPending {
		t.Fatalf("upserted program status = %q, want pending", status)
	}

	after, err := repo.ReviewProgram(ctx, admin, id, ProgramReviewInput{Approved: true, Reason: "核對無誤"})
	if err != nil {
		t.Fatalf("ReviewProgram: %v", err)
	}
	if after.ReviewStatus != ProgramStatusPublished {
		t.Fatalf("reviewed program status = %q, want published", after.ReviewStatus)
	}

	// A second review of an already-published program is rejected.
	if _, err := repo.ReviewProgram(ctx, admin, id, ProgramReviewInput{Approved: true, Reason: "again"}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("re-review error = %v, want ErrInvalidStatus", err)
	}

	history, err := repo.ListProgramHistory(ctx, admin, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 2 {
		t.Fatalf("expected import + publish audit events, got %d", len(history))
	}
}

func TestDeleteProgramOnlyRemovesEmptyPlaceholders(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	admin := dbtest.InsertAdmin(t, ctx, pool)
	plain := dbtest.InsertAccount(t, ctx, pool, "")
	school := dbtest.AnySchoolCode(t, ctx, pool)

	realID := ProgramIdentifier{AcademicYear: 114, SchoolCode: school, ProgramCode: "720"}
	emptyID := ProgramIdentifier{AcademicYear: 114, SchoolCode: school, ProgramCode: "721"}
	weight := 100.0
	batch := ProgramBatchInput{
		Reason: "integration import",
		Items: []ProgramInput{
			{
				AcademicYear:         114,
				SchoolCode:           school,
				ProgramCode:          "720",
				AdmissionProgramName: "有資料學系",
				AdmissionQuota:       3,
				ExamItems:            []ExamItem{{Name: "資料審查", SortOrder: 1, WeightPercent: &weight}},
			},
			{
				AcademicYear:         114,
				SchoolCode:           school,
				ProgramCode:          "721",
				AdmissionProgramName: "空白佔位學系",
				AdmissionQuota:       0,
				ExamItems:            []ExamItem{{Name: "依簡章公告為準", SortOrder: 1, WeightPercent: &weight}},
			},
		},
	}
	if _, err := repo.UpsertPrograms(ctx, admin, batch); err != nil {
		t.Fatalf("UpsertPrograms: %v", err)
	}

	if err := repo.DeleteProgram(ctx, plain, emptyID, "not an admin"); !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("non-admin delete error = %v, want ErrAdminRequired", err)
	}
	if err := repo.DeleteProgram(ctx, admin, realID, "should be refused"); !errors.Is(err, ErrProgramNotDeletable) {
		t.Fatalf("delete of a real program error = %v, want ErrProgramNotDeletable", err)
	}
	if err := repo.DeleteProgram(ctx, admin, emptyID, "cleanup unused placeholder"); err != nil {
		t.Fatalf("DeleteProgram: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM academic_programs WHERE academic_year=114 AND school_code=$1 AND program_code='721'`, school).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("deleted program still present: count = %d", count)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM program_exam_items WHERE academic_year=114 AND school_code=$1 AND program_code='721'`, school).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("deleted program's exam items still present: count = %d", count)
	}
	if err := repo.DeleteProgram(ctx, admin, emptyID, "already gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("re-delete of already-deleted program error = %v, want ErrNotFound", err)
	}
}
