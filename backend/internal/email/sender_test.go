package email

import (
	"context"
	"strings"
	"testing"
)

func TestNewSMTPSenderRequiresTLS(t *testing.T) {
	if _, err := NewSMTPSender(SMTPConfig{Host: "smtp.example.test", From: "noreply@example.test", UseTLS: false}); err == nil {
		t.Fatal("NewSMTPSender() error = nil, want TLS requirement")
	}
}

func TestNewSMTPSenderAllowInsecureForLocalRelay(t *testing.T) {
	// Cleartext is accepted only with the explicit escape hatch.
	if _, err := NewSMTPSender(SMTPConfig{
		Host: "mailhog", Port: 1025, From: "noreply@example.test", UseTLS: false, AllowInsecure: true,
	}); err != nil {
		t.Fatalf("NewSMTPSender(AllowInsecure) error = %v", err)
	}
	// An out-of-range port is still rejected regardless of the flag.
	if _, err := NewSMTPSender(SMTPConfig{
		Host: "mailhog", Port: 99999, From: "noreply@example.test", UseTLS: false, AllowInsecure: true,
	}); err == nil {
		t.Fatal("NewSMTPSender() accepted an invalid port")
	}
}

func TestSMTPSenderRejectsHeaderInjection(t *testing.T) {
	sender, err := NewSMTPSender(SMTPConfig{Host: "smtp.example.test", From: "noreply@example.test", UseTLS: true})
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}
	if err := sender.Send(context.Background(), Message{To: "user@example.test", Subject: "hello\r\nBcc: attacker@example.test"}); err == nil {
		t.Fatal("Send() error = nil, want header injection rejection")
	}
}

func TestSMTPSenderAcceptsMultiLineBody(t *testing.T) {
	// A multi-line body must pass validation (the send then fails at connect,
	// not at "invalid line breaks"). Regression: the body was wrongly rejected
	// for containing newlines, so no email ever left the outbox.
	sender, err := NewSMTPSender(SMTPConfig{Host: "127.0.0.1", Port: 1, From: "noreply@example.test", UseTLS: true})
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}
	err = sender.Send(context.Background(), Message{
		To: "user@example.test", Subject: "STA 密碼重設",
		Text: "有人為這個帳號要求重設密碼。\n請在 30 分鐘內完成。\n\nToken：abc123",
	})
	if err == nil || strings.Contains(err.Error(), "line breaks") {
		t.Fatalf("multi-line body rejected by validation: %v", err)
	}
	if !strings.Contains(err.Error(), "connect SMTP") {
		t.Fatalf("expected a connect failure, got: %v", err)
	}
}

func TestFormatMessageNormalizesBody(t *testing.T) {
	formatted := formatMessage("from@example.test", "to@example.test", Message{Subject: "Subject", Text: "a\nb"})
	if !strings.Contains(formatted, "a\r\nb\r\n") {
		t.Fatalf("formatted message did not normalize line endings: %q", formatted)
	}
}

// A raw non-ASCII Subject header makes some receiving MTAs (observed against
// NTU's mail gateway) require the SMTPUTF8 extension and hard-bounce when the
// next hop doesn't offer it. The header must stay pure ASCII via RFC 2047
// encoding.
func TestFormatMessageEncodesNonASCIISubject(t *testing.T) {
	formatted := formatMessage("from@example.test", "to@example.test", Message{Subject: "STA 帳號啟用", Text: "body"})
	for _, line := range strings.Split(formatted, "\r\n") {
		if !strings.HasPrefix(line, "Subject:") {
			continue
		}
		for _, r := range line {
			if r > 127 {
				t.Fatalf("Subject header contains a raw non-ASCII rune: %q", line)
			}
		}
		if !strings.Contains(line, "=?UTF-8?") {
			t.Fatalf("Subject header was not RFC 2047 encoded: %q", line)
		}
		return
	}
	t.Fatal("no Subject header found in formatted message")
}

// Without In-Reply-To/References, a reply to something the recipient sent
// us shows up in their mail client as an unrelated new email rather than
// threading under the one they sent.
func TestFormatMessageSetsInReplyToAndReferences(t *testing.T) {
	formatted := formatMessage("from@example.test", "to@example.test", Message{
		Subject: "Re: hello", Text: "body", InReplyTo: "<original-123@example.test>",
	})
	if !strings.Contains(formatted, "In-Reply-To: <original-123@example.test>\r\n") {
		t.Fatalf("formatted message is missing In-Reply-To: %q", formatted)
	}
	if !strings.Contains(formatted, "References: <original-123@example.test>\r\n") {
		t.Fatalf("formatted message is missing References: %q", formatted)
	}
}

func TestFormatMessageOmitsInReplyToWhenUnset(t *testing.T) {
	formatted := formatMessage("from@example.test", "to@example.test", Message{Subject: "Subject", Text: "body"})
	if strings.Contains(formatted, "In-Reply-To:") || strings.Contains(formatted, "References:") {
		t.Fatalf("formatted message should not have threading headers when InReplyTo is unset: %q", formatted)
	}
}

func TestFormatMessageWithHTMLProducesMultipartAlternative(t *testing.T) {
	formatted := formatMessage("from@example.test", "to@example.test", Message{
		Subject: "Subject",
		Text:    "plain fallback, no link shown",
		HTML: PasswordResetEmail(PasswordResetEmailData{
			LogoURL:  "https://sta-tw.org/logo.svg",
			ResetURL: "https://sta-tw.org/reset-password?token=SECRET",
		}),
	})
	if !strings.Contains(formatted, "Content-Type: multipart/alternative;") {
		t.Fatalf("expected a multipart/alternative message, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "Content-Type: text/plain; charset=UTF-8") {
		t.Fatal("missing text/plain part")
	}
	if !strings.Contains(formatted, "Content-Type: text/html; charset=UTF-8") {
		t.Fatal("missing text/html part")
	}
	if !strings.Contains(formatted, "SECRET") {
		t.Fatal("the button's URL (with the token) should still be present in the HTML part")
	}
}
