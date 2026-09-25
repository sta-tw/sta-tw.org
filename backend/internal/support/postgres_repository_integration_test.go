//go:build integration

package support

import (
	"context"
	"testing"

	"sta-backend/internal/auth"
	"sta-backend/internal/dbtest"
)

func TestCreateTicketAndAdminReplyDriveOutbox(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	cipher, err := auth.NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewPostgresRepository(pool, cipher, make([]byte, 32), "support@sta.local", "https://sta.local")
	if err != nil {
		t.Fatal(err)
	}

	user := dbtest.InsertAccount(t, ctx, pool, "")
	admin := dbtest.InsertAdmin(t, ctx, pool)

	ticket, err := repo.CreateTicket(ctx, user, CreateTicketInput{
		Category: string(CategoryTechnical),
		Subject:  "無法上傳備審檔案",
		Body:     "上傳時一直失敗，請協助。",
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if ticket.Ticket.Status != string(StatusWaitingStaff) {
		t.Fatalf("new ticket status = %q, want waiting_staff", ticket.Ticket.Status)
	}
	if len(ticket.Messages) != 1 {
		t.Fatalf("new ticket messages = %d, want 1", len(ticket.Messages))
	}

	if _, err := repo.AddAdminMessage(ctx, admin, ticket.Ticket.ID, "已收到，正在查。"); err != nil {
		t.Fatalf("AddAdminMessage: %v", err)
	}

	// The admin reply must be queued for Discord delivery.
	var pending int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM support_discord_outbox
		WHERE ticket_id = $1 AND status IN ('pending', 'failed')
	`, ticket.Ticket.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending == 0 {
		t.Fatal("admin reply did not enqueue a Discord outbox row")
	}

	got, err := repo.GetTicket(ctx, &user, ticket.Ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("ticket messages after admin reply = %d, want 2", len(got.Messages))
	}
}
