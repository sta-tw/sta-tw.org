package accountapplications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// approveRejectKeyboard is the ✅/❌ keyboard attached to a still-pending
// application's card — shared by the initial post and every later edit, so
// an edit that doesn't resend it doesn't silently drop the buttons.
func approveRejectKeyboard(applicationID uuid.UUID) map[string]any {
	return map[string]any{
		"inline_keyboard": [][]map[string]string{
			{
				{"text": "✅ 核准", "callback_data": "account_app:approve:" + applicationID.String()},
				{"text": "❌ 拒絕", "callback_data": "account_app:reject:" + applicationID.String()},
			},
		},
	}
}

// HTTPTelegramNotifier posts review messages directly to the Telegram Bot
// HTTP API. It's used by cmd/api, which is a different process from the
// long-polling cmd/telegram-bot that later receives the approve/reject
// button presses and calls back into the decision API endpoint.
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

// NotifyPendingApplication posts the one message an application's whole
// Telegram thread lives in. The returned headerText is meta only — sender,
// action buttons, validity note — never the actual message content, which
// always lives in the transcript (see Service.renderCard) so an edit never
// ends up showing it twice.
func (n *HTTPTelegramNotifier) NotifyPendingApplication(ctx context.Context, app Application, email string, documents []DocumentLink) (int64, int64, string, error) {
	if !n.Enabled() {
		return 0, 0, "", errors.New("telegram account-application notifier is not configured")
	}
	// Emailing account@ is not an application — /apply (the web form) is the
	// only path that creates one. A cold email is treated as a plain
	// inquiry: no approve/reject buttons, no "this becomes an account" copy.
	// It's still stored via the same Application row so a later reply on
	// the same thread (HandleInboundReply) has something to attach to.
	if app.Source == "email" {
		return n.notifyInquiryEmail(ctx, app, email, documents)
	}

	var text strings.Builder
	fmt.Fprintf(&text, "新的帳號申請（無學校信箱）\n帳號：%s\n聯絡信箱：%s\n", app.RequestedUsername, email)
	text.WriteString("核准後會直接建立帳號（已驗證學生），並寄設定密碼的連結到這個信箱。\n")
	if len(documents) == 0 {
		text.WriteString("（沒有附上佐證檔案）\n")
	} else {
		text.WriteString("佐證檔案：\n")
		for _, doc := range documents {
			fmt.Fprintf(&text, "- %s\n%s\n", doc.Filename, doc.URL)
		}
	}
	text.WriteString("\n連結有效 72 小時。")
	headerText := text.String()

	messageID, err := n.send(ctx, n.chatID, headerText, approveRejectKeyboard(app.ID))
	if err != nil {
		return 0, 0, "", err
	}
	return n.chatID, messageID, headerText, nil
}

func (n *HTTPTelegramNotifier) notifyInquiryEmail(ctx context.Context, app Application, fromEmail string, documents []DocumentLink) (int64, int64, string, error) {
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

	messageID, err := n.send(ctx, n.chatID, headerText, nil)
	if err != nil {
		return 0, 0, "", err
	}
	return n.chatID, messageID, headerText, nil
}

func (n *HTTPTelegramNotifier) send(ctx context.Context, chatID int64, text string, keyboard any) (int64, error) {
	payload := map[string]any{"chat_id": chatID, "text": text}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
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

func (n *HTTPTelegramNotifier) UpdateMessage(ctx context.Context, chatID, messageID int64, text string, applicationID uuid.UUID, showDecisionButtons bool) error {
	if !n.Enabled() {
		return errors.New("telegram account-application notifier is not configured")
	}
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}
	// Telegram's editMessageText removes any existing inline keyboard
	// unless reply_markup is resent with the edit — see the interface doc
	// on TelegramNotifier.UpdateMessage.
	if showDecisionButtons {
		payload["reply_markup"] = approveRejectKeyboard(applicationID)
	}
	return n.apiRequest(ctx, "editMessageText", payload, nil)
}

func (n *HTTPTelegramNotifier) DeleteMessage(ctx context.Context, chatID, messageID int64) error {
	if !n.Enabled() {
		return errors.New("telegram account-application notifier is not configured")
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
