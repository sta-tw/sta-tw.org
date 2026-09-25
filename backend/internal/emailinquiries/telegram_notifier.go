package emailinquiries

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HTTPTelegramNotifier posts inquiry notifications directly to the Telegram
// Bot HTTP API. It's used by cmd/api, which is a different process from the
// long-polling cmd/telegram-bot that later receives a staff member's reply.
type HTTPTelegramNotifier struct {
	botToken string
	chatID   int64
	client   *http.Client
}

func NewHTTPTelegramNotifier(botToken string, chatID int64) *HTTPTelegramNotifier {
	return &HTTPTelegramNotifier{
		botToken: strings.TrimSpace(botToken),
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *HTTPTelegramNotifier) Enabled() bool {
	return n != nil && n.botToken != "" && n.chatID != 0
}

// NotifyInquiry posts the one message an inquiry's whole Telegram thread
// lives in. The returned headerText is meta only — sender, attachments —
// never the actual message content, which always lives in the transcript
// (see Service.renderCard) so an edit never ends up showing it twice.
// It never carries decision buttons — an inquiry isn't reviewed, just
// answered.
func (n *HTTPTelegramNotifier) NotifyInquiry(ctx context.Context, inquiry Inquiry, fromEmail string, documents []DocumentLink) (int64, int64, string, error) {
	if !n.Enabled() {
		return 0, 0, "", errors.New("telegram email-inquiry notifier is not configured")
	}
	var text strings.Builder
	fmt.Fprintf(&text, "收到一封信到帳號申請信箱\n寄件人：%s\n", fromEmail)
	if len(documents) == 0 {
		text.WriteString("（沒有附件）\n")
	} else {
		text.WriteString("附件：\n")
		for _, doc := range documents {
			fmt.Fprintf(&text, "- %s\n%s\n", doc.Filename, doc.URL)
		}
	}
	text.WriteString("\n這是一般詢問，不是帳號申請；申請帳號請引導對方改用 /apply 表單。")
	headerText := text.String()

	messageID, err := n.send(ctx, n.chatID, headerText)
	if err != nil {
		return 0, 0, "", err
	}
	return n.chatID, messageID, headerText, nil
}

func (n *HTTPTelegramNotifier) send(ctx context.Context, chatID int64, text string) (int64, error) {
	payload := map[string]any{"chat_id": chatID, "text": text}
	var result struct {
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := n.apiRequest(ctx, "sendMessage", payload, &result); err != nil {
		return 0, err
	}
	return result.Result.MessageID, nil
}

func (n *HTTPTelegramNotifier) UpdateMessage(ctx context.Context, chatID, messageID int64, text string) error {
	if !n.Enabled() {
		return errors.New("telegram email-inquiry notifier is not configured")
	}
	return n.apiRequest(ctx, "editMessageText", map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}, nil)
}

func (n *HTTPTelegramNotifier) DeleteMessage(ctx context.Context, chatID, messageID int64) error {
	if !n.Enabled() {
		return errors.New("telegram email-inquiry notifier is not configured")
	}
	return n.apiRequest(ctx, "deleteMessage", map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}, nil)
}

func (n *HTTPTelegramNotifier) apiRequest(ctx context.Context, method string, payload map[string]any, destination any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode telegram request: %w", err)
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", n.botToken, method)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := n.client.Do(request)
	if err != nil {
		return fmt.Errorf("call telegram API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", response.StatusCode)
	}
	if destination == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(destination)
}
