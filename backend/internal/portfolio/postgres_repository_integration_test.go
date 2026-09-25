//go:build integration

package portfolio

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"sta-backend/internal/dbtest"
)

func TestPortfolioFileReviewStateMachine(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}

	owner := dbtest.InsertStudent(t, ctx, pool)
	admin := dbtest.InsertAdmin(t, ctx, pool)
	school := dbtest.AnySchoolCode(t, ctx, pool)
	dbtest.InsertPublishedProgram(t, ctx, pool, 114, school, "801")

	appID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO applications (id, account_id, academic_year, school_code, program_code, status, locked_at)
		VALUES ($1, $2, 114, $3, '801', 'confirmed', CURRENT_TIMESTAMP)
	`, appID, owner, school); err != nil {
		t.Fatal(err)
	}

	project, err := repo.CreateProject(ctx, owner, appID, "備審資料")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	file, err := repo.CreateFile(ctx, owner, project.ID, "cv.pdf", "portfolio/cv.pdf", "application/pdf", 1024, "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	if file.Status != FileStatusHidden {
		t.Fatalf("new file status = %q, want hidden", file.Status)
	}

	// A non-admin cannot review, and a hidden (not-yet-submitted) file cannot.
	if _, err := repo.ReviewFile(ctx, owner, file.ID, true, ""); !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("non-admin review error = %v, want ErrNotAdmin", err)
	}
	if _, err := repo.ReviewFile(ctx, admin, file.ID, true, ""); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("review of hidden file error = %v, want ErrInvalidStatus", err)
	}

	if _, err := repo.SubmitForReview(ctx, owner, file.ID); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	reviewed, err := repo.ReviewFile(ctx, admin, file.ID, true, "")
	if err != nil {
		t.Fatalf("ReviewFile approve: %v", err)
	}
	if reviewed.Status != FileStatusPublished {
		t.Fatalf("status after approval = %q, want published", reviewed.Status)
	}

	events, err := repo.ListFileEvents(ctx, owner, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 {
		t.Fatalf("expected submit + approve events, got %d", len(events))
	}
}
