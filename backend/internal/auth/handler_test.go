package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandlerRegistrationLoginAndMe(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	notifier := &fakeEmailNotifier{}
	service.ConfigureEmailVerification(notifier, "")
	handler, err := NewHandler(service, nil)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	registerRecorder := httptest.NewRecorder()
	registerRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"handler-user","email":"handler@example.test","school_email":"handler@ntu.edu.tw"}`))
	registerRequest.RemoteAddr = "192.0.2.20:1234"
	mux.ServeHTTP(registerRecorder, registerRequest)
	if registerRecorder.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", registerRecorder.Code, registerRecorder.Body.String())
	}
	if bytes.Contains(registerRecorder.Body.Bytes(), []byte("handler@example.test")) {
		t.Fatal("registration response should not echo email")
	}
	var registered accountResponse
	if err := json.Unmarshal(registerRecorder.Body.Bytes(), &registered); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	token := notifier.lastTokenFor(registered.Account.ID)
	if token == "" {
		t.Fatalf("no activation token captured for account %s", registered.Account.ID)
	}
	confirmRecorder := httptest.NewRecorder()
	confirmRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset/confirm",
		bytes.NewBufferString(`{"token":"`+token+`","new_password":"correct horse battery staple"}`))
	mux.ServeHTTP(confirmRecorder, confirmRequest)
	if confirmRecorder.Code != http.StatusNoContent {
		t.Fatalf("confirm status = %d, body = %s", confirmRecorder.Code, confirmRecorder.Body.String())
	}

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"handler-user","password":"correct horse battery staple"}`))
	loginRequest.RemoteAddr = "192.0.2.20:1234"
	mux.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRecorder.Code, loginRecorder.Body.String())
	}
	cookies := loginRecorder.Result().Cookies()
	if findCookie(cookies, sessionCookieName) == nil || findCookie(cookies, csrfCookieName) == nil {
		t.Fatalf("login did not set both session and CSRF cookies: %#v", cookies)
	}

	meRecorder := httptest.NewRecorder()
	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(findCookie(cookies, sessionCookieName))
	mux.ServeHTTP(meRecorder, meRequest)
	if meRecorder.Code != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", meRecorder.Code, meRecorder.Body.String())
	}
	var response accountResponse
	if err := json.Unmarshal(meRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode me response: %v", err)
	}
	if response.Account.Username != "handler-user" {
		t.Fatalf("me username = %q", response.Account.Username)
	}
}

func TestHandlerRejectsUnknownJSONFields(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, _ := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	handler, _ := NewHandler(service, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"user","email":"user@example.test","password":"correct horse battery staple","role":"admin"}`))

	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
