package telegrambot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseCommandSupportsBotMention(t *testing.T) {
	command, args, ok := ParseCommand(" /brochure@sta_test_bot 116 001 ")
	if !ok || command != "brochure" || len(args) != 2 || args[0] != "116" || args[1] != "001" {
		t.Fatalf("ParseCommand() = %q, %v, %v", command, args, ok)
	}
}

func TestHandleUpdateHealthAndBrochure(t *testing.T) {
	var sentText string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "telegram.test" {
			if request.URL.Path != "/bot123:test/sendMessage" {
				t.Fatalf("Telegram path = %q", request.URL.Path)
			}
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			sentText = payload.Text
			return testResponse(http.StatusOK, `{"ok":true,"result":{"message_id":7}}`), nil
		}
		if request.URL.Host == "backend.test" {
			switch request.URL.Path {
			case "/healthz":
				return testResponse(http.StatusOK, `{"status":"ok"}`), nil
			case "/api/v1/admissions/brochures/116/001/download":
				return testResponse(http.StatusOK, `{"data":{"academic_year":116,"school_code":"001","original_file_name":"116-001.pdf"},"url":"https://storage.test/signed.pdf","expires_in":300}`), nil
			}
		}
		return testResponse(http.StatusNotFound, `{}`), nil
	})}
	bot, err := New(Config{
		Token:              "123:test",
		BackendBaseURL:     "http://backend.test",
		TelegramAPIBaseURL: "http://telegram.test",
		PollTimeout:        time.Second,
		HTTPClient:         client,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := bot.HandleUpdate(ctx, Update{Message: &Message{Chat: Chat{ID: 42}, Text: "/health"}}); err != nil {
		t.Fatal(err)
	}
	if sentText != "STA API 正常（healthz=ok）。" {
		t.Fatalf("health response = %q", sentText)
	}
	if err := bot.HandleUpdate(ctx, Update{Message: &Message{Chat: Chat{ID: 42}, Text: "/brochure 116 001"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sentText, "116-001.pdf") || !strings.Contains(sentText, "https://storage.test/signed.pdf") {
		t.Fatalf("brochure response = %q", sentText)
	}
}

func TestHandleUpdateHonorsAllowedChatIDs(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		called = true
		return testResponse(http.StatusOK, `{"ok":true,"result":{"message_id":7}}`), nil
	})}
	bot, err := New(Config{
		Token:              "123:test",
		BackendBaseURL:     "http://backend.test",
		TelegramAPIBaseURL: "http://telegram.test",
		AllowedChatIDs:     map[int64]struct{}{42: {}},
		HTTPClient:         client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleUpdate(context.Background(), Update{Message: &Message{Chat: Chat{ID: 99}, Text: "/id"}}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("unauthorized chat caused a Telegram response")
	}
}

func TestConfigFromEnvRejectsInvalidAllowedChatID(t *testing.T) {
	t.Setenv("STA_TELEGRAM_BOT_ALLOWED_CHAT_IDS", "42,nope")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("ConfigFromEnv accepted an invalid chat id")
	}
}

func TestConfigFromEnvLoadsCrossCheckToken(t *testing.T) {
	t.Setenv("STA_TELEGRAM_CROSS_CHECK_TOKEN", "connected-adapter-token")
	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.CrossCheckToken != "connected-adapter-token" {
		t.Fatalf("ConfigFromEnv cross-check token = %q", config.CrossCheckToken)
	}
}

func TestCrossCheckStartBindsPrivateChatAndHidesInternalPercentage(t *testing.T) {
	var sentText string
	var bindAuthorized bool
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "backend.test" {
			bindAuthorized = request.Header.Get("Authorization") == "Bearer service-secret"
			switch request.URL.Path {
			case "/api/v1/internal/telegram-cross-check/bind":
				var input struct {
					TelegramUserID int64 `json:"telegram_user_id"`
					PrivateChatID  int64 `json:"private_chat_id"`
				}
				if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
					t.Fatal(err)
				}
				if input.TelegramUserID != 42 || input.PrivateChatID != 42 {
					t.Fatalf("binding = %#v", input)
				}
				return testResponse(http.StatusNoContent, ``), nil
			case "/api/v1/internal/telegram-cross-check/users/42/dashboard":
				return testResponse(http.StatusOK, `{"data":{"telegram_user_id":42,"notifications_enabled":true,"applications":[{"application_id":"00000000-0000-0000-0000-000000000001","program_identifier":"116-001-001","academic_year":116,"school_code":"001","school_name":"測試大學","program_code":"001","program_name":"資訊工程學系","result_status":"waitlisted","official_rank":3,"current_choice":"low_interest","current_choice_label":"意願偏低"}]}}`), nil
			}
		}
		if request.URL.Host == "telegram.test" && request.URL.Path == "/bot123:test/sendMessage" {
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			sentText = payload.Text
			return testResponse(http.StatusOK, `{"ok":true,"result":{"message_id":9}}`), nil
		}
		return testResponse(http.StatusNotFound, `{}`), nil
	})}
	bot, err := New(Config{
		Token:              "123:test",
		CrossCheckToken:    "service-secret",
		BackendBaseURL:     "http://backend.test",
		TelegramAPIBaseURL: "http://telegram.test",
		HTTPClient:         client,
	})
	if err != nil {
		t.Fatal(err)
	}
	message := &Message{From: &User{ID: 42}, Chat: Chat{ID: 42, Type: "private"}, Text: "/start"}
	if err := bot.HandleUpdate(context.Background(), Update{Message: message}); err != nil {
		t.Fatal(err)
	}
	if !bindAuthorized {
		t.Fatal("cross-check request did not use the service bearer")
	}
	if !strings.Contains(sentText, "備取第 3 名") || !strings.Contains(sentText, "意願偏低") {
		t.Fatalf("start text = %q", sentText)
	}
	if strings.Contains(sentText, "%") || strings.Contains(sentText, "20") {
		t.Fatalf("start text leaked an internal percentage: %q", sentText)
	}
}

func TestCrossCheckCallbackSendsChoiceLabelWithoutNumericValue(t *testing.T) {
	inquiryID := uuid.New()
	var backendPayload map[string]any
	var answered, confirmation bool
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "backend.test" && request.URL.Path == "/api/v1/internal/telegram-cross-check/respond" {
			if err := json.NewDecoder(request.Body).Decode(&backendPayload); err != nil {
				t.Fatal(err)
			}
			return testResponse(http.StatusOK, `{"data":{"application_id":"00000000-0000-0000-0000-000000000001","program_identifier":"116-001-001","school_name":"測試大學","program_name":"資訊工程學系","choice":"low_interest","choice_label":"意願偏低"}}`), nil
		}
		if request.URL.Host == "telegram.test" {
			switch request.URL.Path {
			case "/bot123:test/answerCallbackQuery":
				answered = true
				return testResponse(http.StatusOK, `{"ok":true,"result":true}`), nil
			case "/bot123:test/editMessageReplyMarkup":
				return testResponse(http.StatusOK, `{"ok":true,"result":{"message_id":7}}`), nil
			case "/bot123:test/sendMessage":
				confirmation = true
				return testResponse(http.StatusOK, `{"ok":true,"result":{"message_id":8}}`), nil
			}
		}
		return testResponse(http.StatusNotFound, `{}`), nil
	})}
	bot, err := New(Config{
		Token:              "123:test",
		CrossCheckToken:    "service-secret",
		BackendBaseURL:     "http://backend.test",
		TelegramAPIBaseURL: "http://telegram.test",
		HTTPClient:         client,
	})
	if err != nil {
		t.Fatal(err)
	}
	callback := CallbackQuery{
		ID:   "callback-1",
		From: User{ID: 42},
		Message: &Message{
			MessageID: 7,
			Chat:      Chat{ID: 42, Type: "private"},
		},
		Data: "wc:" + inquiryID.String() + ":low",
	}
	if err := bot.HandleUpdate(context.Background(), Update{CallbackQuery: &callback}); err != nil {
		t.Fatal(err)
	}
	if backendPayload["choice"] != "low_interest" || backendPayload["callback_id"] != "callback-1" {
		t.Fatalf("response payload = %#v", backendPayload)
	}
	if _, exists := backendPayload["value"]; exists {
		t.Fatalf("response payload exposed an internal numeric value: %#v", backendPayload)
	}
	if !answered || !confirmation {
		t.Fatalf("answered = %v, confirmation = %v", answered, confirmation)
	}
}

func TestDispatchDeliveryUsesOpinionLabels(t *testing.T) {
	inquiryID := uuid.New()
	deliveryID := uuid.New()
	var keyboard map[string]any
	markedSent := false
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "backend.test" {
			switch {
			case request.URL.Path == "/api/v1/internal/telegram-cross-check/outbox/claim":
				body := `{"data":[{"id":"` + deliveryID.String() + `","telegram_user_id":42,"chat_id":42,"inquiry_id":"` + inquiryID.String() + `","application_id":"00000000-0000-0000-0000-000000000001","program_identifier":"116-001-001","academic_year":116,"school_name":"測試大學","program_name":"資訊工程學系","result_status":"waitlisted","official_rank":3,"inquiry_round":"result_released"}]}`
				return testResponse(http.StatusOK, body), nil
			case strings.HasSuffix(request.URL.Path, "/sent"):
				markedSent = true
				return testResponse(http.StatusNoContent, ``), nil
			}
		}
		if request.URL.Host == "telegram.test" && request.URL.Path == "/bot123:test/sendMessage" {
			var payload struct {
				Text        string         `json:"text"`
				ReplyMarkup map[string]any `json:"reply_markup"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(payload.Text, "%") {
				t.Fatalf("delivery text leaked a percentage: %q", payload.Text)
			}
			keyboard = payload.ReplyMarkup
			return testResponse(http.StatusOK, `{"ok":true,"result":{"message_id":15}}`), nil
		}
		return testResponse(http.StatusNotFound, `{}`), nil
	})}
	bot, err := New(Config{
		Token:              "123:test",
		CrossCheckToken:    "service-secret",
		BackendBaseURL:     "http://backend.test",
		TelegramAPIBaseURL: "http://telegram.test",
		HTTPClient:         client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bot.dispatchDeliveries(context.Background()); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(keyboard)
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"完全不考慮", "意願偏低", "還在考慮", "傾向選擇", "高度有意願", "確定選擇"} {
		if !strings.Contains(string(encoded), label) {
			t.Fatalf("keyboard %s missing label %q", encoded, label)
		}
	}
	if strings.Contains(string(encoded), "%") || !markedSent {
		t.Fatalf("keyboard = %s, markedSent = %v", encoded, markedSent)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
