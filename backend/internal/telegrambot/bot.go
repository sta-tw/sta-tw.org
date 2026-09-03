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
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBackendBaseURL = "http://localhost:8080"
	defaultTelegramAPIURL = "https://api.telegram.org"
	defaultPollTimeout    = 25 * time.Second
	maxPollTimeout        = 50 * time.Second
	requestBodyLimit      = 2 << 20
)

var (
	ErrNotConfigured        = errors.New("telegram bot is not configured")
	ErrBrochureNotPublished = errors.New("published brochure was not found")
	ErrCrossCheckAuth       = errors.New("Telegram cross-check API authentication failed")
)

// Config contains the deliberately small surface of the Telegram test bot.
// It does not share the lounge outbox worker: the bot can therefore be tested
// with only a Telegram token and a running STA API. When the optional
// cross-check service token is set, it also delivers cross-check inquiries.
type Config struct {
	Token string
	// CrossCheckToken authenticates requests to the optional Telegram
	// cross-check adapter mounted by cmd/api.
	CrossCheckToken    string
	BackendBaseURL     string
	TelegramAPIBaseURL string
	AllowedChatIDs     map[int64]struct{}
	PollTimeout        time.Duration
	HTTPClient         *http.Client
}

// ConfigFromEnv loads the Telegram-only test bot configuration. The existing
// STA_TELEGRAM_BOT_TOKEN is intentionally reused so the same secret manager
// entry can be used by the later production worker.
func ConfigFromEnv() (Config, error) {
	config := Config{
		Token:              strings.TrimSpace(os.Getenv("STA_TELEGRAM_BOT_TOKEN")),
		CrossCheckToken:    strings.TrimSpace(os.Getenv("STA_TELEGRAM_CROSS_CHECK_TOKEN")),
		BackendBaseURL:     valueOrDefault("STA_TELEGRAM_BACKEND_BASE_URL", defaultBackendBaseURL),
		TelegramAPIBaseURL: valueOrDefault("STA_TELEGRAM_API_BASE_URL", defaultTelegramAPIURL),
		PollTimeout:        defaultPollTimeout,
	}
	if raw := strings.TrimSpace(os.Getenv("STA_TELEGRAM_POLL_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 || parsed > maxPollTimeout {
			return Config{}, fmt.Errorf("STA_TELEGRAM_POLL_TIMEOUT must be between 1s and 50s")
		}
		config.PollTimeout = parsed
	}
	allowed, err := parseAllowedChatIDs(os.Getenv("STA_TELEGRAM_BOT_ALLOWED_CHAT_IDS"))
	if err != nil {
		return Config{}, err
	}
	config.AllowedChatIDs = allowed
	return config, nil
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parseAllowedChatIDs(raw string) (map[int64]struct{}, error) {
	allowed := make(map[int64]struct{})
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("STA_TELEGRAM_BOT_ALLOWED_CHAT_IDS contains an invalid chat id")
		}
		allowed[id] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, nil
	}
	return allowed, nil
}

type Bot struct {
	token              string
	crossCheckToken    string
	backendBaseURL     string
	telegramAPIBaseURL string
	allowedChatIDs     map[int64]struct{}
	pollTimeout        time.Duration
	client             *http.Client
}

func New(config Config) (*Bot, error) {
	token := strings.TrimSpace(config.Token)
	if token == "" || strings.ContainsAny(token, "/?#\r\n\t ") {
		return nil, ErrNotConfigured
	}
	backendBaseURL, err := validateBaseURL(config.BackendBaseURL, "backend")
	if err != nil {
		return nil, err
	}
	telegramAPIBaseURL, err := validateBaseURL(config.TelegramAPIBaseURL, "Telegram API")
	if err != nil {
		return nil, err
	}
	pollTimeout := config.PollTimeout
	if pollTimeout == 0 {
		pollTimeout = defaultPollTimeout
	}
	if pollTimeout <= 0 || pollTimeout > maxPollTimeout {
		return nil, errors.New("Telegram poll timeout must be between 1s and 50s")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: pollTimeout + 10*time.Second}
	}
	allowed := make(map[int64]struct{}, len(config.AllowedChatIDs))
	for id := range config.AllowedChatIDs {
		if id != 0 {
			allowed[id] = struct{}{}
		}
	}
	return &Bot{
		token:              token,
		crossCheckToken:    strings.TrimSpace(config.CrossCheckToken),
		backendBaseURL:     backendBaseURL,
		telegramAPIBaseURL: telegramAPIBaseURL,
		allowedChatIDs:     allowed,
		pollTimeout:        pollTimeout,
		client:             client,
	}, nil
}

func validateBaseURL(raw, label string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%s base URL is invalid", label)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%s base URL must use HTTP or HTTPS", label)
	}
	return raw, nil
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
}

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

type telegramEnvelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

type apiError struct {
	code int
}

func (e *apiError) Error() string {
	return fmt.Sprintf("Telegram API error %d", e.code)
}

type BotInfo struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// Verify calls getMe so a startup smoke test fails immediately when the token
// or Telegram endpoint is wrong, instead of waiting for the first poll.
func (b *Bot) Verify(ctx context.Context) (BotInfo, error) {
	if b == nil {
		return BotInfo{}, ErrNotConfigured
	}
	var info BotInfo
	if err := b.apiRequest(ctx, "getMe", nil, &info); err != nil {
		return BotInfo{}, err
	}
	if !info.IsBot || info.ID == 0 {
		return BotInfo{}, errors.New("Telegram API returned an invalid bot identity")
	}
	return info, nil
}

// Run starts Telegram long polling. A webhook must be removed before this
// process is started; Telegram does not deliver updates to both mechanisms.
func (b *Bot) Run(ctx context.Context) error {
	if b == nil {
		return ErrNotConfigured
	}
	var offset int64
	for {
		if b.crossCheckToken != "" {
			if err := b.dispatchDeliveries(ctx); err != nil && ctx.Err() != nil {
				return ctx.Err()
			}
		}
		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var telegramErr *apiError
			if errors.As(err, &telegramErr) && (telegramErr.code == http.StatusUnauthorized || telegramErr.code == http.StatusConflict) {
				return err
			}
			if err := waitForRetry(ctx, 2*time.Second); err != nil {
				return err
			}
			continue
		}
		for _, update := range updates {
			if err := b.HandleUpdate(ctx, update); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
		}
	}
}

func waitForRetry(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	payload := map[string]any{"timeout": int(b.pollTimeout / time.Second)}
	if offset > 0 {
		payload["offset"] = offset
	}
	var updates []Update
	if err := b.apiRequest(ctx, "getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

// ParseCommand parses both /brochure 116 001 and /brochure@sta_bot 116 001.
func ParseCommand(text string) (command string, args []string, ok bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return "", nil, false
	}
	raw := strings.TrimPrefix(fields[0], "/")
	if raw == "" {
		return "", nil, false
	}
	if at := strings.IndexByte(raw, '@'); at >= 0 {
		raw = raw[:at]
	}
	if raw == "" {
		return "", nil, false
	}
	return strings.ToLower(raw), fields[1:], true
}

// HandleUpdate handles only explicit slash commands. Regular chat messages
// are ignored so this test bot does not unexpectedly mirror private content.
func (b *Bot) HandleUpdate(ctx context.Context, update Update) error {
	if b == nil {
		return nil
	}
	if update.CallbackQuery != nil {
		return b.handleCallback(ctx, *update.CallbackQuery)
	}
	if update.Message == nil {
		return nil
	}
	message := update.Message
	if len(b.allowedChatIDs) > 0 {
		if _, ok := b.allowedChatIDs[message.Chat.ID]; !ok {
			return nil
		}
	}
	command, args, ok := ParseCommand(message.Text)
	if !ok {
		return nil
	}
	var response string
	switch command {
	case "start":
		if b.crossCheckToken == "" {
			response = "Telegram 交叉查榜 adapter 目前未啟用。\n\n" + helpText()
		} else {
			response = b.crossCheckCommandText(ctx, message, command)
		}
	case "help":
		response = helpText()
	case "list", "pending", "status", "history", "stop":
		if b.crossCheckToken == "" {
			response = "Telegram 交叉查榜 adapter 目前未啟用。"
		} else {
			response = b.crossCheckCommandText(ctx, message, command)
		}
	case "id":
		response = fmt.Sprintf("此聊天室 ID：%d\n把這個值填入 STA_TELEGRAM_BOT_ALLOWED_CHAT_IDS，可限制 Bot 只回應這個聊天室。", message.Chat.ID)
	case "health":
		if err := b.checkBackendHealth(ctx); err != nil {
			response = "STA API 目前無法連線或尚未就緒。"
		} else {
			response = "STA API 正常（healthz=ok）。"
		}
	case "brochure":
		response = b.brochureText(ctx, args)
	default:
		response = "不支援這個指令。\n\n" + helpText()
	}
	return b.sendText(ctx, message.Chat.ID, response)
}

func helpText() string {
	return "STA Telegram 服務\n\n" +
		"/start：啟用交叉查榜私人通知\n" +
		"/list：查看我的交叉查榜狀態\n" +
		"/pending：查看待回覆的查榜項目\n" +
		"/status：查看我的查榜狀態\n" +
		"/history：查看我的意見歷程\n" +
		"/stop：停止後續私人通知\n" +
		"/id：顯示目前聊天室 ID\n" +
		"/health：檢查 STA API\n" +
		"/brochure <學年度> <學校編號>：取得已上架簡章連結\n" +
		"例如：/brochure 116 001"
}

func (b *Bot) sendText(ctx context.Context, chatID int64, text string) error {
	_, err := b.sendMessage(ctx, chatID, text, nil)
	return err
}

func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string, replyMarkup any) (int64, error) {
	if chatID == 0 {
		return 0, errors.New("Telegram message has no chat id")
	}
	text = truncateRunes(text, 4096)
	payload := map[string]any{"chat_id": chatID, "text": text}
	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}
	var result struct {
		MessageID int64 `json:"message_id"`
	}
	if err := b.apiRequest(ctx, "sendMessage", payload, &result); err != nil {
		return 0, err
	}
	if result.MessageID <= 0 {
		return 0, errors.New("Telegram API returned an invalid message id")
	}
	return result.MessageID, nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func (b *Bot) checkBackendHealth(ctx context.Context) error {
	status, body, err := b.backendRequest(ctx, http.MethodGet, "/healthz", nil)
	if err != nil || status < http.StatusOK || status >= http.StatusMultipleChoices {
		return errors.New("STA API health check failed")
	}
	var payload struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Status != "ok" {
		return errors.New("STA API health response is invalid")
	}
	return nil
}

type brochureDownloadResponse struct {
	Data struct {
		OriginalFileName string `json:"original_file_name"`
		AcademicYear     int    `json:"academic_year"`
		SchoolCode       string `json:"school_code"`
	} `json:"data"`
	URL       string `json:"url"`
	ExpiresIn int    `json:"expires_in"`
}

func (b *Bot) brochureText(ctx context.Context, args []string) string {
	if len(args) != 2 || !threeDigitNumber(args[0], 100, 999) || !threeDigitNumber(args[1], 1, 999) {
		return "格式：/brochure <三位數學年度> <三位數學校編號>\n例如：/brochure 116 001"
	}
	path := fmt.Sprintf("/api/v1/admissions/brochures/%s/%s/download", args[0], args[1])
	status, body, err := b.backendRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "STA API 目前無法連線。"
	}
	if status == http.StatusNotFound {
		return "目前找不到這間學校已上架的簡章。"
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "簡章服務目前無法完成查詢。"
	}
	var payload brochureDownloadResponse
	if json.Unmarshal(body, &payload) != nil || payload.URL == "" {
		return "簡章回應格式不完整，請回到控制台檢查。"
	}
	return fmt.Sprintf("簡章已上架：%s\n有效時間：%d 秒\n下載連結：\n%s", payload.Data.OriginalFileName, payload.ExpiresIn, payload.URL)
}

func threeDigitNumber(raw string, minimum, maximum int) bool {
	if len(raw) != 3 {
		return false
	}
	value, err := strconv.Atoi(raw)
	return err == nil && value >= minimum && value <= maximum
}

func (b *Bot) backendRequest(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, b.backendBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, errors.New("STA API request could not be created")
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(request)
	if err != nil {
		return 0, nil, errors.New("STA API request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, requestBodyLimit))
	if err != nil {
		return response.StatusCode, nil, errors.New("STA API response could not be read")
	}
	return response.StatusCode, responseBody, nil
}

func (b *Bot) apiRequest(ctx context.Context, method string, payload any, destination any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return errors.New("Telegram request could not be encoded")
		}
	}
	endpoint := b.telegramAPIBaseURL + "/bot" + url.PathEscape(b.token) + "/" + method
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("Telegram request could not be created")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		// Do not expose the underlying url.Error: it may include the bot token.
		return errors.New("Telegram API request failed")
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, requestBodyLimit))
	if readErr != nil {
		return errors.New("Telegram API response could not be read")
	}
	var envelope telegramEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("Telegram API returned status %d", response.StatusCode)
		}
		return errors.New("Telegram API response is invalid")
	}
	if !envelope.OK {
		if envelope.ErrorCode > 0 {
			return &apiError{code: envelope.ErrorCode}
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return &apiError{code: response.StatusCode}
		}
		return errors.New("Telegram API rejected the request")
	}
	if destination != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, destination); err != nil {
			return errors.New("Telegram API result is invalid")
		}
	}
	return nil
}
