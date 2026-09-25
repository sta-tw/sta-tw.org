package telegramcrosscheck

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sta-backend/internal/auth"
)

func TestInternalBindRequiresServiceToken(t *testing.T) {
	repository := &fakeRepository{}
	handler, err := NewHandler(
		&auth.Service{}, repository, &fakeWillingnessWriter{},
		mustTestCipher(t), make([]byte, 32), "service-secret", true,
	)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	payload, _ := json.Marshal(BindInput{TelegramUserID: 123, PrivateChatID: 123})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/internal/telegram-cross-check/bind", bytes.NewReader(payload))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/internal/telegram-cross-check/bind", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer service-secret")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.bound.TelegramUserID != 123 || repository.bound.PrivateChatID != 123 {
		t.Fatalf("bound input = %#v", repository.bound)
	}
}

func mustTestCipher(t *testing.T) *auth.FieldCipher {
	t.Helper()
	cipher, err := auth.NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}
