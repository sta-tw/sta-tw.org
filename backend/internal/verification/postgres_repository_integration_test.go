//go:build integration

package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"sta-backend/internal/dbtest"
)

func TestEmailChallengeConsumePromotesAccountAtomically(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	account := dbtest.InsertAccount(t, ctx, pool, "")
	school := dbtest.AnySchoolCode(t, ctx, pool)

	req, err := repo.CreateEmailRequest(ctx, account, CreateRequestInput{
		AcademicYear: 114,
		SchoolCode:   school,
		SchoolEmail:  "s@example.edu.tw",
	}, []byte("cipher"), []byte("lookup-"+account.String()))
	if err != nil {
		t.Fatalf("CreateEmailRequest: %v", err)
	}

	now := time.Now().UTC()
	codeHash := []byte("correct-code-hash-32-bytes-------")
	if err := repo.CreateEmailChallenge(ctx, req.ID, codeHash, now.Add(15*time.Minute)); err != nil {
		t.Fatalf("CreateEmailChallenge: %v", err)
	}

	// Wrong code: rejected, request stays pending, account not promoted.
	if _, err := repo.ConsumeEmailCode(ctx, account, req.ID, []byte("wrong"), now, now.Add(365*24*time.Hour)); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("wrong code error = %v, want ErrInvalidCode", err)
	}
	var status, identity string
	if err := pool.QueryRow(ctx, `SELECT status FROM verification_requests WHERE id=$1`, req.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("request status after wrong code = %q", status)
	}

	// Correct code: request approved, account becomes student, event recorded.
	if _, err := repo.ConsumeEmailCode(ctx, account, req.ID, codeHash, now, now.Add(365*24*time.Hour)); err != nil {
		t.Fatalf("ConsumeEmailCode: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM verification_requests WHERE id=$1`, req.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "approved" {
		t.Fatalf("request status after correct code = %q", status)
	}
	if err := pool.QueryRow(ctx, `SELECT identity_status FROM accounts WHERE id=$1`, account).Scan(&identity); err != nil {
		t.Fatal(err)
	}
	if identity != "student" {
		t.Fatalf("account identity_status = %q, want student", identity)
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_events WHERE request_id=$1 AND to_status='approved'`, req.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events == 0 {
		t.Fatal("no approval event recorded")
	}
}
