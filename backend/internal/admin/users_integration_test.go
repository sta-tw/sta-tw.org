//go:build integration

package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"sta-backend/internal/dbtest"
)

func TestSuspendReinstateAndForceLogout(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	admin := dbtest.InsertAdmin(t, ctx, pool)
	target := dbtest.InsertAccount(t, ctx, pool, "")
	bystander := dbtest.InsertAccount(t, ctx, pool, "")

	seedSession := func(accountID uuid.UUID) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO account_sessions (account_id, token_hash, csrf_token_hash, expires_at)
			VALUES ($1, $2, $3, CURRENT_TIMESTAMP + INTERVAL '30 days')
		`, accountID, []byte("tok-"+uuid.NewString()), []byte("csrf-"+uuid.NewString())); err != nil {
			t.Fatal(err)
		}
	}
	seedSession(target)
	seedSession(target)
	seedSession(bystander)

	live := func(accountID uuid.UUID) int {
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM account_sessions
			WHERE account_id = $1 AND revoked_at IS NULL`, accountID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Suspend revokes the target's sessions, records the reason, and leaves
	// other accounts alone.
	revoked, status, err := suspendAccount(ctx, pool, target, admin, "spam")
	if err != nil || status != "suspended" || revoked != 2 {
		t.Fatalf("suspendAccount = %d, %q, %v; want 2, suspended, nil", revoked, status, err)
	}
	if live(target) != 0 {
		t.Fatalf("target still has %d live sessions after suspend", live(target))
	}
	if live(bystander) != 1 {
		t.Fatalf("bystander sessions touched: %d", live(bystander))
	}
	var (
		accStatus string
		reason    *string
		by        *uuid.UUID
	)
	if err := pool.QueryRow(ctx, `
		SELECT account_status, suspension_reason, suspended_by FROM accounts WHERE id = $1`, target).
		Scan(&accStatus, &reason, &by); err != nil {
		t.Fatal(err)
	}
	if accStatus != "suspended" || reason == nil || *reason != "spam" || by == nil || *by != admin {
		t.Fatalf("suspension state = %q / %v / %v", accStatus, reason, by)
	}

	// A second suspend is a conflict, not a silent success.
	if _, _, err := suspendAccount(ctx, pool, target, admin, "again"); err != errAccountNotActive {
		t.Fatalf("double suspend err = %v, want errAccountNotActive", err)
	}

	// An admin account is protected.
	if _, _, err := suspendAccount(ctx, pool, admin, admin, "no"); err != errAccountIsAdmin {
		t.Fatalf("suspend admin err = %v, want errAccountIsAdmin", err)
	}

	// Missing account.
	if _, _, err := suspendAccount(ctx, pool, uuid.New(), admin, "no"); err != errAccountNotFound {
		t.Fatalf("suspend missing err = %v, want errAccountNotFound", err)
	}

	// Reinstate clears the suspension columns.
	if err := reinstateAccount(ctx, pool, target, admin, "appeal upheld"); err != nil {
		t.Fatalf("reinstateAccount: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT account_status, suspension_reason, suspended_by FROM accounts WHERE id = $1`, target).
		Scan(&accStatus, &reason, &by); err != nil {
		t.Fatal(err)
	}
	if accStatus != "active" || reason != nil || by != nil {
		t.Fatalf("post-reinstate state = %q / %v / %v", accStatus, reason, by)
	}
	if err := reinstateAccount(ctx, pool, target, admin, "again"); err != errAccountNotSuspended {
		t.Fatalf("double reinstate err = %v, want errAccountNotSuspended", err)
	}

	// force-logout revokes without changing status.
	seedSession(target)
	seedSession(target)
	n, err := forceLogoutAccount(ctx, pool, target, admin, "device lost")
	if err != nil || n != 2 {
		t.Fatalf("forceLogoutAccount = %d, %v; want 2, nil", n, err)
	}
	if live(target) != 0 {
		t.Fatalf("force-logout left %d live sessions", live(target))
	}
	if err := pool.QueryRow(ctx, `SELECT account_status FROM accounts WHERE id = $1`, target).Scan(&accStatus); err != nil {
		t.Fatal(err)
	}
	if accStatus != "active" {
		t.Fatalf("force-logout changed status to %q", accStatus)
	}
	if _, err := forceLogoutAccount(ctx, pool, uuid.New(), admin, "x"); err != errAccountNotFound {
		t.Fatalf("force-logout missing err = %v, want errAccountNotFound", err)
	}

	// Three account.* audit rows were written for the target.
	var auditRows int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		WHERE entity_type = 'account' AND entity_key = $1
		  AND action IN ('account.suspended', 'account.reinstated', 'account.force_logout')`,
		target.String()).Scan(&auditRows); err != nil {
		t.Fatal(err)
	}
	if auditRows != 3 {
		t.Fatalf("account audit rows = %d, want 3", auditRows)
	}
}
