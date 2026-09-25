package mailintake

import (
	"encoding/base64"
	"strings"
	"testing"
)

const nestedMultipartEmail = "" +
	"--outer\r\n" +
	"Content-Type: multipart/alternative; boundary=inner\r\n" +
	"\r\n" +
	"--inner\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"帳號名稱: my-cool-user\r\n這是我的申請說明。\r\n" +
	"--inner\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>不應該被當成 body</p>\r\n" +
	"--inner--\r\n" +
	"--outer\r\n" +
	"Content-Type: application/pdf; name=\"proof.pdf\"\r\n" +
	"Content-Disposition: attachment; filename=\"proof.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"JVBERi0xLjQK\r\n" + // "%PDF-1.4\n" base64
	"--outer--\r\n"

func TestParseBodyNestedMultipartWithAttachment(t *testing.T) {
	body, attachments := ParseBody("multipart/mixed; boundary=outer", "", strings.NewReader(nestedMultipartEmail))

	if !strings.Contains(body, "帳號名稱: my-cool-user") {
		t.Fatalf("expected plain-text body to be extracted from nested multipart/alternative, got %q", body)
	}
	if strings.Contains(body, "不應該被當成") {
		t.Fatalf("html alternative should not have been used as the body: %q", body)
	}
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Filename != "proof.pdf" {
		t.Fatalf("expected filename proof.pdf, got %q", attachments[0].Filename)
	}
	if string(attachments[0].Data) != "%PDF-1.4\n" {
		t.Fatalf("expected decoded base64 attachment data, got %q", attachments[0].Data)
	}
}

func TestStripQuotedReplyRemovesGmailQuoteBlock(t *testing.T) {
	body := "測試回覆 這一段式不重要得資訊\r\n" +
		"\r\n" +
		"<account@mail.sta-tw.org> 於 2026年9月18日週五 上午1:00寫道：\r\n" +
		">\r\n" +
		"> 原本的信件內容在這裡\r\n"
	got := StripQuotedReply(body)
	if got != "測試回覆 這一段式不重要得資訊" {
		t.Fatalf("expected quoted block stripped, got %q", got)
	}
}

func TestStripQuotedReplyRemovesEnglishWroteHeader(t *testing.T) {
	body := "thanks, got it\n\nOn Fri, Sep 18, 2026 at 1:00 AM <account@mail.sta-tw.org> wrote:\n> original message\n"
	got := StripQuotedReply(body)
	if got != "thanks, got it" {
		t.Fatalf("expected quoted block stripped, got %q", got)
	}
}

func TestStripQuotedReplyLeavesPlainBodyAlone(t *testing.T) {
	body := "沒有引用內容的普通回覆"
	if got := StripQuotedReply(body); got != body {
		t.Fatalf("expected body unchanged, got %q", got)
	}
}

// TestParseBodySinglePartBase64 covers Gmail's "Forward" feature, which
// sends a non-multipart message with Content-Transfer-Encoding: base64 on
// the top-level header — without decoding it there, the raw base64 text was
// being used as the body verbatim.
func TestParseBodySinglePartBase64(t *testing.T) {
	raw := base64.StdEncoding.EncodeToString([]byte("沒有收到驗證信"))
	body, attachments := ParseBody("text/plain; charset=UTF-8", "base64", strings.NewReader(raw))
	if body != "沒有收到驗證信" {
		t.Fatalf("expected decoded base64 body, got %q", body)
	}
	if len(attachments) != 0 {
		t.Fatalf("expected no attachments, got %d", len(attachments))
	}
}
