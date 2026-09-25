//go:build integration

package admissions

import (
	"context"
	"testing"

	"sta-backend/internal/dbtest"
)

// TestCreateBrochureThenReviewPublishes exercises CreateBrochure ->
// ReviewBrochure end to end against a real Postgres connection over pgx's
// extended query protocol. changeBrochureStatus's UPDATE used the same bind
// parameter ($3) in two expressions (SET target and inside a CASE); pgx
// infers a type per occurrence, and Postgres rejected the mismatch
// ("inconsistent types deduced for parameter $3") — a failure mode psql's
// simple query protocol never surfaces, which is how this shipped
// unexercised.
func TestCreateBrochureThenReviewPublishes(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	admin := dbtest.InsertAdmin(t, ctx, pool)
	school := dbtest.AnySchoolCode(t, ctx, pool)

	document, _, err := repo.CreateBrochure(ctx, admin, BrochureDocumentInput{
		AcademicYear:     116,
		SchoolCode:       school,
		OriginalFileName: "brochure.pdf",
		StorageKey:       "brochures/uploads/integration-test.pdf",
		MIMEType:         "application/pdf",
		FileSizeBytes:    1024,
		SHA256:           "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	if err != nil {
		t.Fatalf("CreateBrochure: %v", err)
	}
	if document.ReviewStatus != "pending" {
		t.Fatalf("created brochure status = %q, want pending", document.ReviewStatus)
	}

	published, err := repo.ReviewBrochure(ctx, admin, 116, school, true, "整合測試核准")
	if err != nil {
		t.Fatalf("ReviewBrochure(approved): %v", err)
	}
	if published.ReviewStatus != "published" {
		t.Fatalf("reviewed brochure status = %q, want published", published.ReviewStatus)
	}
	if published.PublishedAt == nil {
		t.Fatal("expected published_at to be set")
	}
}
