//go:build integration

package brochurediscovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"sta-backend/internal/dbtest"
)

func TestCycleLifecycleAndClaim(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	admin := dbtest.InsertAdmin(t, ctx, pool)
	plain := dbtest.InsertAccount(t, ctx, pool, "")

	if _, _, err := repo.CreateCycle(ctx, plain, 114); !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("non-admin CreateCycle error = %v, want ErrAdminRequired", err)
	}

	cycle, tasks, err := repo.CreateCycle(ctx, admin, 114)
	if err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}
	if cycle.Status != CycleDraft {
		t.Fatalf("new cycle status = %q, want draft", cycle.Status)
	}
	if tasks == 0 {
		t.Fatal("CreateCycle seeded 0 tasks; expected the roster to fan out")
	}

	// A draft cycle hands out no work.
	if _, err := repo.ClaimNextSystem(ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claim on draft cycle error = %v, want ErrNotFound", err)
	}

	started, err := repo.StartCycle(ctx, admin, 114)
	if err != nil {
		t.Fatalf("StartCycle: %v", err)
	}
	if started.Status != CycleActive {
		t.Fatalf("started cycle status = %q, want active", started.Status)
	}

	// Now a system agent can lease a task, and it flips to 'searching'.
	task, err := repo.ClaimNextSystem(ctx, time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextSystem: %v", err)
	}
	if task.SchoolCode == "" {
		t.Fatal("ClaimNextSystem returned an empty task")
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM brochure_discovery_tasks WHERE academic_year=114 AND school_code=$1`, task.SchoolCode).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != StatusSearching {
		t.Fatalf("claimed task status = %q, want searching", status)
	}

	// Starting an already-active cycle is rejected.
	if _, err := repo.StartCycle(ctx, admin, 114); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("re-start error = %v, want ErrInvalidStatus", err)
	}
}
