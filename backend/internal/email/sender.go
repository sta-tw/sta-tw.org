package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Message struct {
	To      string
	Subject string
	Text    string
}

type Sender interface {
	Send(context.Context, Message) error
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
	// AllowInsecure permits a cleartext (no STARTTLS) connection. It exists
	// only for a local relay such as MailHog and must never be set in
	// production; config.Load ignores STA_SMTP_ALLOW_INSECURE outside dev.
	AllowInsecure bool
}

type SMTPSender struct {
	config SMTPConfig
}

func NewSMTPSender(config SMTPConfig) (*SMTPSender, error) {
	config.Host = strings.TrimSpace(config.Host)
	config.Username = strings.TrimSpace(config.Username)
	config.From = strings.TrimSpace(config.From)
	if config.Host == "" || config.From == "" {
		return nil, errors.New("SMTP host and from address are required")
	}
	if config.Port == 0 {
		config.Port = 587
	}
	if config.Port < 1 || config.Port > 65535 {
		return nil, errors.New("SMTP requires a valid port")
	}
	if !config.UseTLS && !config.AllowInsecure {
		return nil, errors.New("SMTP requires TLS (set AllowInsecure only for a local relay)")
	}
	if _, err := parseMailbox(config.From); err != nil {
		return nil, fmt.Errorf("SMTP from address is invalid: %w", err)
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, errors.New("SMTP username and password must be configured together")
	}
	return &SMTPSender{config: config}, nil
}

func (s *SMTPSender) Send(ctx context.Context, message Message) error {
	if s == nil {
		return errors.New("SMTP sender is not configured")
	}
	to, err := parseMailbox(message.To)
	if err != nil {
		return fmt.Errorf("recipient address is invalid: %w", err)
	}
	from, err := parseMailbox(s.config.From)
	if err != nil {
		return fmt.Errorf("sender address is invalid: %w", err)
	}
	// Only the Subject becomes a header, so only it must be free of CR/LF; the
	// body is multi-line by nature and formatMessage normalises its line
	// endings, while net/smtp's DATA writer handles dot-stuffing.
	if hasHeaderInjection(message.Subject) {
		return errors.New("email subject contains invalid line breaks")
	}
	if hasNULOrControlEscape(message.Text) {
		return errors.New("email body contains disallowed control characters")
	}
	if strings.TrimSpace(message.Subject) == "" || len(message.Subject) > 200 || len(message.Text) > 1<<20 {
		return errors.New("email content is invalid")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	address := net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var connection net.Conn
	if s.config.Port == 465 {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}}
		connection, err = tlsDialer.DialContext(ctx, "tcp", address)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("connect SMTP: %w", err)
	}
	client, err := smtp.NewClient(connection, s.config.Host)
	if err != nil {
		_ = connection.Close()
		return fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()
	if s.config.Port != 465 && !(!s.config.UseTLS && s.config.AllowInsecure) {
		supported, _ := client.Extension("STARTTLS")
		if !supported {
			return errors.New("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	if s.config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)); err != nil {
			return fmt.Errorf("authenticate SMTP: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open SMTP data: %w", err)
	}
	if _, err := io.WriteString(writer, formatMessage(from, to, message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close SMTP data: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("finish SMTP delivery: %w", err)
	}
	return nil
}

func parseMailbox(value string) (string, error) {
	if hasHeaderInjection(value) {
		return "", errors.New("address contains invalid line breaks")
	}
	address, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil || address.Address == "" {
		return "", errors.New("address cannot be parsed")
	}
	return address.Address, nil
}

func hasHeaderInjection(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

// hasNULOrControlEscape rejects a NUL or a bare ESC in the body; ordinary
// whitespace (\n \r \t) is fine and gets normalised by formatMessage.
func hasNULOrControlEscape(value string) bool {
	return strings.ContainsAny(value, "\x00\x1b")
}

func formatMessage(from, to string, message Message) string {
	body := strings.ReplaceAll(message.Text, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	return "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + message.Subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" + body + "\r\n"
}
