//go:build integration

package ingestion

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"sta-backend/internal/admissions"
	"sta-backend/internal/dbtest"
)

func TestReviewCandidateAtomicallyCreatesPendingProgram(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repository, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}

	adminID := dbtest.InsertAdmin(t, ctx, pool)
	runID := uuid.New()
	candidateID := uuid.New()
	sha := strings.Repeat(strings.ReplaceAll(runID.String(), "-", ""), 2)
	if _, err := pool.Exec(ctx, `
		INSERT INTO brochure_extraction_runs
			(id, academic_year, school_code, source_sha256_hex, processor_version, status)
		VALUES ($1,997,'001',$2,'integration-test','pending_review')
	`, runID, sha); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO brochure_extraction_candidates (id, run_id, program_code, extracted_data)
		VALUES ($1,$2,'998','{}'::jsonb)
	`, candidateID, runID); err != nil {
		t.Fatal(err)
	}

	// An incomplete program payload must roll the whole review back.
	_, err = repository.ReviewCandidate(ctx, adminID, candidateID, ReviewInput{
		Approved: true,
		Program:  &admissions.ProgramInput{AdmissionProgramName: "不完整資料"},
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid approval error = %v, want ErrInvalid", err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT review_status FROM brochure_extraction_candidates WHERE id=$1`, candidateID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != CandidatePending {
		t.Fatalf("candidate status after rollback = %q", status)
	}

	weight := 100.0
	page := 12
	candidate, err := repository.ReviewCandidate(ctx, adminID, candidateID, ReviewInput{
		Approved: true,
		Reason:   "已核對隔離測試簡章",
		Program: &admissions.ProgramInput{
			AdmissionProgramName: "測試學系",
			AdmissionQuota:       2,
			ExamItems:            []admissions.ExamItem{{Name: "資料審查", SortOrder: 1, WeightPercent: &weight}},
			BrochureURL:          "https://admission.example.edu.tw/997.pdf",
			SourcePage:           &page,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.ReviewStatus != CandidateApproved {
		t.Fatalf("candidate status = %q", candidate.ReviewStatus)
	}
	var programStatus string
	if err := pool.QueryRow(ctx, `
		SELECT review_status FROM academic_programs
		WHERE academic_year=997 AND school_code='001' AND program_code='998'
	`).Scan(&programStatus); err != nil {
		t.Fatal(err)
	}
	if programStatus != admissions.ProgramStatusPending {
		t.Fatalf("program status = %q", programStatus)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM brochure_extraction_runs WHERE id=$1`, runID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != RunStatusApproved {
		t.Fatalf("run status = %q", status)
	}
}
