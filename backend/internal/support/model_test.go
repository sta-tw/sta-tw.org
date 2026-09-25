package support

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCreateTicket(t *testing.T) {
	valid := CreateTicketInput{Category: string(CategoryResults), Subject: "查榜問題", Body: "請協助確認我的結果。"}
	if err := ValidateCreateTicket(valid); err != nil {
		t.Fatalf("ValidateCreateTicket(valid) error = %v", err)
	}
	invalidCategory := valid
	invalidCategory.Category = "password"
	if err := ValidateCreateTicket(invalidCategory); err == nil {
		t.Fatal("ValidateCreateTicket(invalid category) error = nil")
	}
	invalidBody := valid
	invalidBody.Body = "bad\x00body"
	if err := ValidateCreateTicket(invalidBody); err == nil {
		t.Fatal("ValidateCreateTicket(control body) error = nil")
	}
}

func TestValidateStatus(t *testing.T) {
	if err := ValidateStatus(StatusClosed, false); err != nil {
		t.Fatalf("user close status error = %v", err)
	}
	if err := ValidateStatus(StatusSpam, false); err == nil {
		t.Fatal("user spam status error = nil")
	}
	if err := ValidateStatus(StatusSpam, true); err != nil {
		t.Fatalf("admin spam status error = %v", err)
	}
}

func TestTicketNumberAndDiscordContent(t *testing.T) {
	if got := ticketNumber(12); got != "T-000012" {
		t.Fatalf("ticketNumber(12) = %q", got)
	}
	content := discordMessageContent(DiscordOutboxTask{TicketNumber: 12, Body: strings.Repeat("x", 2000), AuthorType: "user"})
	if len(content) > 2000 || !strings.Contains(content, "使用者：") || !strings.Contains(content, "完整內容請至 STA 查看") {
		t.Fatalf("discord content was not safely truncated: length=%d content=%q", len(content), content)
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	body := []byte(`{"channel_id":"123"}`)
	if VerifyWebhookSignature("secret", body, "") {
		t.Fatal("empty signature was accepted")
	}
	if VerifyWebhookSignature("secret", body, "deadbeef") {
		t.Fatal("invalid signature was accepted")
	}
}

func TestValidateExternalEmailMessage(t *testing.T) {
	valid := ExternalEmailMessage{
		ExternalMessageID: "provider-123",
		TicketNumber:      "T-000123",
		From:              "客服人員 <support@example.edu.tw>",
		Subject:           "Ticket 回覆",
		Body:              "請登入平台查看回覆。",
	}
	if err := ValidateExternalEmailMessage(valid); err != nil {
		t.Fatalf("ValidateExternalEmailMessage(valid) error = %v", err)
	}
	invalid := valid
	invalid.TicketNumber = "T-123"
	if err := ValidateExternalEmailMessage(invalid); err == nil {
		t.Fatal("ValidateExternalEmailMessage(invalid ticket) error = nil")
	}
}

func TestValidateAttachments(t *testing.T) {
	valid := []AttachmentInput{{
		OriginalName:  "screenshot.png",
		StorageKey:    "support/account/file",
		MIMEType:      "image/png",
		FileSizeBytes: 128,
		SHA256:        strings.Repeat("a", 64),
	}}
	if err := ValidateAttachments(valid); err != nil {
		t.Fatalf("ValidateAttachments(valid) error = %v", err)
	}
	invalid := valid
	invalid[0].StorageKey = "../escape"
	if err := ValidateAttachments(invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ValidateAttachments(invalid key) = %v, want invalid input", err)
	}
	tooMany := make([]AttachmentInput, MaxAttachmentCount+1)
	if err := ValidateAttachments(tooMany); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ValidateAttachments(too many) = %v, want invalid input", err)
	}
}
