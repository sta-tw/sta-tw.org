package chat

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDiscordSenderCreate(t *testing.T) {
	sender := &DiscordSender{
		Token:     "discord-secret",
		ChannelID: "123456",
		Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost || request.URL.Path != "/api/v10/channels/123456/messages" {
				t.Fatalf("unexpected Discord request: %s %s", request.Method, request.URL.Path)
			}
			if request.Header.Get("Authorization") != "Bot discord-secret" {
				t.Fatalf("Authorization header was not set safely")
			}
			body, _ := io.ReadAll(request.Body)
			if string(body) != `{"content":"hello"}` {
				t.Fatalf("body = %s", body)
			}
			return jsonResponse(http.StatusOK, `{"id":"discord-message-1"}`), nil
		})},
	}

	messageID, err := sender.Send(context.Background(), OutboxTask{TargetPlatform: PlatformDiscord, Operation: OperationCreate, Body: "hello"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if messageID != "discord-message-1" {
		t.Fatalf("messageID = %q", messageID)
	}
}

func TestTelegramSenderCreate(t *testing.T) {
	sender := &TelegramSender{
		Token:  "telegram-secret",
		ChatID: "-100123",
		Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/sendMessage") {
				t.Fatalf("unexpected Telegram request: %s %s", request.Method, request.URL.Path)
			}
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), `"chat_id":"-100123"`) || !strings.Contains(string(body), `"text":"hello"`) {
				t.Fatalf("body = %s", body)
			}
			return jsonResponse(http.StatusOK, `{"ok":true,"result":{"message_id":42}}`), nil
		})},
	}

	messageID, err := sender.Send(context.Background(), OutboxTask{TargetPlatform: PlatformTelegram, Operation: OperationCreate, Body: "hello"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if messageID != "42" {
		t.Fatalf("messageID = %q", messageID)
	}
}

func TestPlatformSenderRejectsMissingExternalID(t *testing.T) {
	sender := &DiscordSender{Token: "secret", ChannelID: "123", Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("request should not be sent")
		return nil, nil
	})}}
	if _, err := sender.Send(context.Background(), OutboxTask{TargetPlatform: PlatformDiscord, Operation: OperationEdit}); err == nil {
		t.Fatal("Send() error = nil, want missing external id error")
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
