package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTurnstileVerifierChecksActionAndHostname(t *testing.T) {
	verifier, err := NewTurnstileVerifier("test-secret", "sta-tw.org")
	if err != nil {
		t.Fatal(err)
	}
	verifier.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.FormValue("secret") != "test-secret" || r.FormValue("response") != "test-token" || r.FormValue("remoteip") != "203.0.113.42" {
			t.Errorf("unexpected siteverify request")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"success":true,"action":"login","hostname":"sta-tw.org"}`)), Header: make(http.Header)}, nil
	})}
	if err := verifier.Verify(context.Background(), "test-token", "login", "203.0.113.42"); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if err := verifier.Verify(context.Background(), "test-token", "signup", "203.0.113.42"); !errors.Is(err, ErrTurnstileInvalid) {
		t.Fatalf("wrong action error = %v", err)
	}
	verifier.hostname = "other.example"
	if err := verifier.Verify(context.Background(), "test-token", "login", "203.0.113.42"); !errors.Is(err, ErrTurnstileInvalid) {
		t.Fatalf("wrong hostname error = %v", err)
	}
	if err := verifier.Verify(context.Background(), "", "login", "203.0.113.42"); !errors.Is(err, ErrTurnstileInvalid) {
		t.Fatalf("missing token error = %v", err)
	}
}

func TestTurnstileVerifierFailsClosedOnServiceError(t *testing.T) {
	verifier, _ := NewTurnstileVerifier("test-secret", "sta-tw.org")
	verifier.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network unavailable")
	})}
	if err := verifier.Verify(context.Background(), "token", "login", ""); !errors.Is(err, ErrTurnstileUnavailable) {
		t.Fatalf("unavailable service error = %v", err)
	}
}
