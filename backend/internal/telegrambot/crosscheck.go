package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/telegramcrosscheck"
)

type backendStatusError struct {
	status int
}

func (err *backendStatusError) Error() string {
	return fmt.Sprintf("STA API returned status %d", err.status)
}

type dashboardEnvelope struct {
	Data telegramcrosscheck.Dashboard `json:"data"`
}

type historyEnvelope struct {
	Data []telegramcrosscheck.HistoryEvent `json:"data"`
}

type responseEnvelope struct {
	Data telegramcrosscheck.RespondResult `json:"data"`
}

type deliveriesEnvelope struct {
	Data []telegramcrosscheck.Delivery `json:"data"`
}

func (b *Bot) crossCheckCommandText(ctx context.Context, message *Message, command string) string {
	telegramUserID, chatID, ok := privateIdentity(message)
	if !ok {
		return "交叉查榜包含個人資料，請直接開啟 Bot 的私人聊天室使用。"
	}
	switch command {
	case "start":
		status, err := b.crossCheckRequest(ctx, http.MethodPost, "/api/v1/internal/telegram-cross-check/bind", telegramcrosscheck.BindInput{
			TelegramUserID: telegramUserID,
			PrivateChatID:  chatID,
		}, nil)
		if err != nil {
			if status == http.StatusNotFound {
				return "你的 Telegram ID 尚未列入這次測試名單，請把 /id 顯示的數字交給管理人員。"
			}
			return "目前無法啟用交叉查榜通知，請稍後再試。"
		}
		dashboard, err := b.loadDashboard(ctx, telegramUserID)
		if err != nil {
			return "私人通知已啟用，但目前無法讀取查榜名單，請稍後使用 /list。"
		}
		return "私人通知已啟用。\n\n" + formatDashboard(dashboard, false)
	case "list", "status":
		dashboard, err := b.loadDashboard(ctx, telegramUserID)
		if err != nil {
			return dashboardErrorText(err)
		}
		return formatDashboard(dashboard, false)
	case "pending":
		dashboard, err := b.loadDashboard(ctx, telegramUserID)
		if err != nil {
			return dashboardErrorText(err)
		}
		return formatDashboard(dashboard, true)
	case "history":
		events, err := b.loadHistory(ctx, telegramUserID)
		if err != nil {
			return dashboardErrorText(err)
		}
		return formatHistory(events)
	case "stop":
		status, err := b.crossCheckRequest(ctx, http.MethodPost, "/api/v1/internal/telegram-cross-check/disable", map[string]int64{
			"telegram_user_id": telegramUserID,
		}, nil)
		if err != nil {
			if status == http.StatusNotFound {
				return "目前沒有可停用的私人通知設定。"
			}
			return "目前無法停用私人通知，請稍後再試。"
		}
		return "已停止後續私人通知。你仍可使用 /start 重新啟用。"
	default:
		return helpText()
	}
}

func privateIdentity(message *Message) (int64, int64, bool) {
	if message == nil || message.From == nil || message.From.ID <= 0 || message.Chat.ID <= 0 {
		return 0, 0, false
	}
	if message.Chat.Type != "private" || message.Chat.ID != message.From.ID {
		return 0, 0, false
	}
	return message.From.ID, message.Chat.ID, true
}

func (b *Bot) loadDashboard(ctx context.Context, telegramUserID int64) (telegramcrosscheck.Dashboard, error) {
	var envelope dashboardEnvelope
	path := "/api/v1/internal/telegram-cross-check/users/" + strconv.FormatInt(telegramUserID, 10) + "/dashboard"
	_, err := b.crossCheckRequest(ctx, http.MethodGet, path, nil, &envelope)
	return envelope.Data, err
}

func (b *Bot) loadHistory(ctx context.Context, telegramUserID int64) ([]telegramcrosscheck.HistoryEvent, error) {
	var envelope historyEnvelope
	path := "/api/v1/internal/telegram-cross-check/users/" + strconv.FormatInt(telegramUserID, 10) + "/history?limit=20"
	_, err := b.crossCheckRequest(ctx, http.MethodGet, path, nil, &envelope)
	return envelope.Data, err
}

func dashboardErrorText(err error) string {
	var statusError *backendStatusError
	if errors.As(err, &statusError) && statusError.status == http.StatusNotFound {
		return "尚未啟用交叉查榜私人通知，請先使用 /start。"
	}
	return "目前無法讀取交叉查榜資料，請稍後再試。"
}

func formatDashboard(dashboard telegramcrosscheck.Dashboard, pendingOnly bool) string {
	title := "我的交叉查榜狀態"
	if pendingOnly {
		title = "尚待回覆的查榜項目"
	}
	sections := make([]string, 0, len(dashboard.Applications))
	for _, application := range dashboard.Applications {
		if pendingOnly && application.PendingInquiry == nil {
			continue
		}
		result := resultStatusText(application.ResultStatus, application.OfficialRank)
		choice := "尚未回覆"
		if application.CurrentChoiceLabel != "" {
			choice = application.CurrentChoiceLabel
		}
		lines := []string{
			fmt.Sprintf("%s %s", application.SchoolName, application.ProgramName),
			fmt.Sprintf("校系代碼：%s", application.ProgramIdentifier),
			"查榜結果：" + result,
			"目前意見：" + choice,
		}
		if application.PendingInquiry != nil {
			lines = append(lines, "待回覆："+inquiryRoundText(application.PendingInquiry.Round))
			if application.PendingInquiry.ResponseDeadline != nil {
				lines = append(lines, "回覆期限："+application.PendingInquiry.ResponseDeadline.In(time.Local).Format("2006-01-02 15:04"))
			}
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	if len(sections) == 0 {
		if pendingOnly {
			return title + "\n\n目前沒有待回覆項目。"
		}
		return title + "\n\n目前沒有已設定的測試校系。"
	}
	return title + "\n\n" + strings.Join(sections, "\n\n")
}

func formatHistory(events []telegramcrosscheck.HistoryEvent) string {
	if len(events) == 0 {
		return "我的回覆歷程\n\n目前還沒有回覆紀錄。"
	}
	lines := make([]string, 0, len(events))
	for _, event := range events {
		lines = append(lines, fmt.Sprintf(
			"%s｜%s %s｜%s｜%s",
			event.CreatedAt.In(time.Local).Format("2006-01-02 15:04"),
			event.SchoolName,
			event.ProgramName,
			inquiryRoundText(event.InquiryRound),
			event.ChoiceLabel,
		))
	}
	return "我的回覆歷程\n\n" + strings.Join(lines, "\n")
}

func resultStatusText(status string, rank *int) string {
	switch status {
	case "admitted":
		if rank != nil {
			return fmt.Sprintf("正取第 %d 名", *rank)
		}
		return "正取"
	case "waitlisted":
		if rank != nil {
			return fmt.Sprintf("備取第 %d 名", *rank)
		}
		return "備取"
	case "rejected":
		return "未錄取"
	case "unknown":
		return "待確認"
	default:
		return "官方結果尚未發布"
	}
}

func inquiryRoundText(round string) string {
	switch round {
	case "result_released":
		return "榜單公布後意見確認"
	case "acceptance_deadline":
		return "報到截止前意見確認"
	default:
		return "意見確認"
	}
}

var callbackChoices = map[string]telegramcrosscheck.Choice{
	"no":   telegramcrosscheck.ChoiceNotConsidering,
	"low":  telegramcrosscheck.ChoiceLowInterest,
	"mid":  telegramcrosscheck.ChoiceConsidering,
	"lean": telegramcrosscheck.ChoiceLeaningYes,
	"high": telegramcrosscheck.ChoiceHighInterest,
	"yes":  telegramcrosscheck.ChoiceDefinite,
}

func callbackKeyboard(inquiryID uuid.UUID) map[string]any {
	button := func(label, code string) map[string]string {
		return map[string]string{"text": label, "callback_data": "wc:" + inquiryID.String() + ":" + code}
	}
	return map[string]any{"inline_keyboard": [][]map[string]string{
		{button("完全不考慮", "no"), button("意願偏低", "low")},
		{button("還在考慮", "mid"), button("傾向選擇", "lean")},
		{button("高度有意願", "high"), button("確定選擇", "yes")},
	}}
}

func parseWillingnessCallback(data string) (uuid.UUID, telegramcrosscheck.Choice, bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 || parts[0] != "wc" {
		return uuid.Nil, "", false
	}
	inquiryID, err := uuid.Parse(parts[1])
	choice, exists := callbackChoices[parts[2]]
	if err != nil || !exists {
		return uuid.Nil, "", false
	}
	return inquiryID, choice, true
}

func (b *Bot) handleCallback(ctx context.Context, callback CallbackQuery) error {
	if callback.ID == "" {
		return nil
	}
	if b.crossCheckToken == "" {
		return b.answerCallback(ctx, callback.ID, "交叉查榜功能尚未設定。", true)
	}
	if callback.Message == nil || callback.From.ID <= 0 || callback.Message.Chat.Type != "private" || callback.Message.Chat.ID != callback.From.ID {
		return b.answerCallback(ctx, callback.ID, "請在 Bot 私人聊天室回覆。", true)
	}
	if len(b.allowedChatIDs) > 0 {
		if _, ok := b.allowedChatIDs[callback.Message.Chat.ID]; !ok {
			return nil
		}
	}
	inquiryID, choice, ok := parseWillingnessCallback(callback.Data)
	if !ok {
		return b.answerCallback(ctx, callback.ID, "這個按鈕已失效，請使用 /pending 重新確認。", true)
	}
	var envelope responseEnvelope
	status, err := b.crossCheckRequest(ctx, http.MethodPost, "/api/v1/internal/telegram-cross-check/respond", telegramcrosscheck.RespondInput{
		TelegramUserID: callback.From.ID,
		InquiryID:      inquiryID,
		Choice:         choice,
		CallbackID:     callback.ID,
	}, &envelope)
	if err != nil {
		message := "目前無法記錄，請稍後再試。"
		if status == http.StatusNotFound {
			message = "這筆詢問已失效或超過回覆期限。"
		}
		return b.answerCallback(ctx, callback.ID, message, true)
	}
	if err := b.answerCallback(ctx, callback.ID, "已記錄："+envelope.Data.ChoiceLabel, false); err != nil {
		return err
	}
	_ = b.apiRequest(ctx, "editMessageReplyMarkup", map[string]any{
		"chat_id":    callback.Message.Chat.ID,
		"message_id": callback.Message.MessageID,
		"reply_markup": map[string]any{
			"inline_keyboard": []any{},
		},
	}, nil)
	return b.sendText(ctx, callback.Message.Chat.ID, fmt.Sprintf(
		"已記錄「%s %s」的意見：%s\n可使用 /history 查看自己的回覆歷程。",
		envelope.Data.SchoolName,
		envelope.Data.ProgramName,
		envelope.Data.ChoiceLabel,
	))
}

func (b *Bot) answerCallback(ctx context.Context, callbackID, message string, alert bool) error {
	var result bool
	return b.apiRequest(ctx, "answerCallbackQuery", map[string]any{
		"callback_query_id": callbackID,
		"text":              truncateRunes(message, 200),
		"show_alert":        alert,
	}, &result)
}

func (b *Bot) dispatchDeliveries(ctx context.Context) error {
	var envelope deliveriesEnvelope
	_, err := b.crossCheckRequest(ctx, http.MethodPost, "/api/v1/internal/telegram-cross-check/outbox/claim", telegramcrosscheck.ClaimInput{Limit: 25}, &envelope)
	if err != nil {
		return err
	}
	for _, delivery := range envelope.Data {
		text := formatDelivery(delivery)
		messageID, sendErr := b.sendMessage(ctx, delivery.ChatID, text, callbackKeyboard(delivery.InquiryID))
		if sendErr != nil {
			retryable := true
			failureMessage := "Telegram API request failed"
			var telegramError *apiError
			if errors.As(sendErr, &telegramError) {
				failureMessage = fmt.Sprintf("Telegram API error %d", telegramError.code)
				if telegramError.code == http.StatusBadRequest || telegramError.code == http.StatusForbidden || telegramError.code == http.StatusUnauthorized {
					retryable = false
				}
			}
			if _, markErr := b.crossCheckRequest(ctx, http.MethodPost,
				"/api/v1/internal/telegram-cross-check/outbox/"+url.PathEscape(delivery.ID.String())+"/failed",
				telegramcrosscheck.FailedInput{Error: failureMessage, Retryable: retryable}, nil); markErr != nil {
				return markErr
			}
			continue
		}
		if _, err := b.crossCheckRequest(ctx, http.MethodPost,
			"/api/v1/internal/telegram-cross-check/outbox/"+url.PathEscape(delivery.ID.String())+"/sent",
			telegramcrosscheck.SentInput{TelegramMessageID: messageID}, nil); err != nil {
			return err
		}
	}
	return nil
}

func formatDelivery(delivery telegramcrosscheck.Delivery) string {
	lines := []string{
		"交叉查榜意見確認",
		"",
		fmt.Sprintf("校系：%s %s", delivery.SchoolName, delivery.ProgramName),
		"校系代碼：" + delivery.ProgramIdentifier,
		"查榜結果：" + resultStatusText(delivery.ResultStatus, delivery.OfficialRank),
		"詢問時點：" + inquiryRoundText(delivery.InquiryRound),
	}
	if delivery.ResponseDeadline != nil {
		lines = append(lines, "回覆期限："+delivery.ResponseDeadline.In(time.Local).Format("2006-01-02 15:04"))
	}
	lines = append(lines, "", "請按下最接近你目前想法的選項。之後仍可依新的詢問再次調整。")
	return strings.Join(lines, "\n")
}

func (b *Bot) crossCheckRequest(ctx context.Context, method, path string, payload any, destination any) (int, error) {
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
	request.Header.Set("Authorization", "Bearer "+b.crossCheckToken)
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
	if response.StatusCode == http.StatusUnauthorized {
		return response.StatusCode, ErrCrossCheckAuth
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response.StatusCode, &backendStatusError{status: response.StatusCode}
	}
	if destination != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, destination); err != nil {
			return response.StatusCode, errors.New("STA API response is invalid")
		}
	}
	return response.StatusCode, nil
}
