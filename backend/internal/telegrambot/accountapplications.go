package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type accountApplicationDecisionResult struct {
	Data struct {
		Status string `json:"status"`
	} `json:"data"`
}

// handleAccountApplicationCallback handles the ✅/❌ buttons on an account
// application review message (posted directly by cmd/api, not by this bot's
// own outbox), calling back into cmd/api to record the decision and then
// editing the message so a second admin can't double-approve it.
func (b *Bot) handleAccountApplicationCallback(ctx context.Context, callback CallbackQuery) error {
	if b.accountApplicationReviewToken == "" {
		return b.answerCallback(ctx, callback.ID, "帳號申請審核功能尚未設定。", true)
	}
	if callback.Message == nil {
		return b.answerCallback(ctx, callback.ID, "找不到原始訊息。", true)
	}
	parts := strings.SplitN(callback.Data, ":", 3)
	if len(parts) != 3 {
		return b.answerCallback(ctx, callback.ID, "這個按鈕已失效。", true)
	}
	action, applicationID := parts[1], parts[2]
	if action != "approve" && action != "reject" {
		return b.answerCallback(ctx, callback.ID, "這個按鈕已失效。", true)
	}

	var result accountApplicationDecisionResult
	status, err := b.accountApplicationRequest(ctx, http.MethodPost, "/api/v1/internal/account-applications/"+applicationID+"/decision",
		map[string]string{"action": action}, &result)
	if err != nil {
		message := "目前無法處理，請稍後再試。"
		switch {
		case status == http.StatusNotFound:
			message = "找不到這筆申請。"
		case status == http.StatusConflict:
			message = "這筆申請已經被處理過了。"
		}
		return b.answerCallback(ctx, callback.ID, message, true)
	}

	label := "已核准"
	if action == "reject" {
		label = "已拒絕"
	}
	if err := b.answerCallback(ctx, callback.ID, label, false); err != nil {
		return err
	}
	reviewer := callback.From.Username
	if reviewer == "" {
		reviewer = callback.From.FirstName
	}
	return b.apiRequest(ctx, "editMessageText", map[string]any{
		"chat_id":    callback.Message.Chat.ID,
		"message_id": callback.Message.MessageID,
		"text":       callback.Message.Text + "\n\n— " + label + "（by " + reviewer + "）",
	}, nil)
}

// handleAccountApplicationReplyMessage forwards a staff member's plain-text
// reply (typed as a Telegram reply to the application's card message) to
// cmd/api, which emails it to the applicant and folds the reply into the
// card's transcript (deleting this raw message in the process — see
// Service.ReplyByTelegram). A 404 means the replied-to message isn't the
// card (this chat may be used for other conversation too), so that's
// silently ignored rather than surfaced as an error.
func (b *Bot) handleAccountApplicationReplyMessage(ctx context.Context, message Message) error {
	if b.accountApplicationReviewToken == "" || message.ReplyToMessage == nil {
		return nil
	}
	body := strings.TrimSpace(message.Text)
	if body == "" {
		return nil
	}
	staffName := "staff"
	if message.From != nil {
		if message.From.Username != "" {
			staffName = message.From.Username
		} else if message.From.FirstName != "" {
			staffName = message.From.FirstName
		}
	}
	status, err := b.accountApplicationRequest(ctx, http.MethodPost, "/api/v1/internal/account-applications/reply-by-telegram",
		map[string]any{
			"chat_id":          message.Chat.ID,
			"message_id":       message.ReplyToMessage.MessageID,
			"staff_message_id": message.MessageID,
			"staff_name":       staffName,
			"body":             body,
		}, nil)
	if err != nil && status != http.StatusNotFound {
		return b.sendText(ctx, message.Chat.ID, "回覆寄送失敗，請稍後再試。")
	}
	return nil
}

func (b *Bot) accountApplicationRequest(ctx context.Context, method, path string, payload any, destination any) (int, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return 0, errors.New("STA API request could not be encoded")
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, b.backendBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, errors.New("STA API request could not be created")
	}
	request.Header.Set("Authorization", "Bearer "+b.accountApplicationReviewToken)
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(request)
	if err != nil {
		return 0, errors.New("STA API request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, requestBodyLimit))
	if err != nil {
		return response.StatusCode, errors.New("STA API response could not be read")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var errorBody struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(responseBody, &errorBody); err == nil && errorBody.Error.Code != "" {
			return response.StatusCode, errors.New(errorBody.Error.Code)
		}
		return response.StatusCode, &backendStatusError{status: response.StatusCode}
	}
	if destination != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, destination); err != nil {
			return response.StatusCode, errors.New("STA API response is invalid")
		}
	}
	return response.StatusCode, nil
}
