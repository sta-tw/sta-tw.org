//go:build integration

package applications

import (
	"context"
	"errors"
	"testing"

	"sta-backend/internal/admissions"
	"sta-backend/internal/dbtest"
)

func TestCreateConfirmedLocksProgramsAndBootstrapsForum(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}

	student := dbtest.InsertStudent(t, ctx, pool)
	school := dbtest.AnySchoolCode(t, ctx, pool)
	dbtest.InsertPublishedProgram(t, ctx, pool, 114, school, "701")

	ids := []admissions.ProgramIdentifier{{AcademicYear: 114, SchoolCode: school, ProgramCode: "701"}}
	apps, err := repo.CreateConfirmed(ctx, student, ids)
	if err != nil {
		t.Fatalf("CreateConfirmed: %v", err)
	}
	if len(apps) != 1 || apps[0].Status != "confirmed" {
		t.Fatalf("unexpected applications: %#v", apps)
	}

	// Forum spaces for the year and the program must exist after confirmation.
	var spaces int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM forum_spaces
		WHERE (space_type = 'annual' AND academic_year = 114)
		   OR (space_type = 'school_program' AND academic_year = 114 AND school_code = $1 AND program_code = '701')
	`, school).Scan(&spaces); err != nil {
		t.Fatal(err)
	}
	if spaces < 2 {
		t.Fatalf("expected annual + school_program forum spaces, found %d", spaces)
	}

	// An unknown program is rejected and nothing is written.
	_, err = repo.CreateConfirmed(ctx, student, []admissions.ProgramIdentifier{{AcademicYear: 114, SchoolCode: school, ProgramCode: "999"}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown program error = %v, want ErrNotFound", err)
	}

	list, err := repo.ListByAccount(ctx, student)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("ListByAccount returned %d, want 1 (no partial writes)", len(list))
	}
}

func TestIsAdmin(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	admin := dbtest.InsertAdmin(t, ctx, pool)
	plain := dbtest.InsertAccount(t, ctx, pool, "")

	if ok, err := repo.IsAdmin(ctx, admin); err != nil || !ok {
		t.Fatalf("IsAdmin(admin) = %v, %v", ok, err)
	}
	if ok, err := repo.IsAdmin(ctx, plain); err != nil || ok {
		t.Fatalf("IsAdmin(plain) = %v, %v", ok, err)
	}
}
