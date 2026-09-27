package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrTurnstileInvalid     = errors.New("turnstile verification failed")
	ErrTurnstileUnavailable = errors.New("turnstile service unavailable")
)

const turnstileSiteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type TurnstileVerifier struct {
	secret   string
	hostname string
	client   *http.Client
	endpoint string
}

func NewTurnstileVerifier(secret, hostname string) (*TurnstileVerifier, error) {
	secret = strings.TrimSpace(secret)
	hostnames := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, rawHostname := range strings.Split(hostname, ",") {
		allowedHostname := strings.ToLower(strings.TrimSpace(rawHostname))
		if allowedHostname == "" || strings.ContainsAny(allowedHostname, "/: ") {
			return nil, ErrNotConfigured
		}
		if _, ok := seen[allowedHostname]; ok {
			continue
		}
		seen[allowedHostname] = struct{}{}
		hostnames = append(hostnames, allowedHostname)
	}
	if secret == "" || len(hostnames) == 0 {
		return nil, ErrNotConfigured
	}
	return &TurnstileVerifier{
		secret: secret, hostname: strings.Join(hostnames, ","),
		client: &http.Client{Timeout: 5 * time.Second}, endpoint: turnstileSiteverifyURL,
	}, nil
}

func (v *TurnstileVerifier) Verify(ctx context.Context, token, action, remoteIP string) error {
	if v == nil || v.client == nil {
		return ErrTurnstileUnavailable
	}
	if token == "" || len(token) > 2048 {
		return ErrTurnstileInvalid
	}
	form := url.Values{"secret": {v.secret}, "response": {token}}
	if net.ParseIP(remoteIP) != nil {
		form.Set("remoteip", remoteIP)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrTurnstileUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := v.client.Do(request)
	if err != nil {
		return ErrTurnstileUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrTurnstileUnavailable
	}
	var result struct {
		Success  bool   `json:"success"`
		Action   string `json:"action"`
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result); err != nil {
		return fmt.Errorf("%w: invalid response", ErrTurnstileUnavailable)
	}
	if !result.Success || result.Action != action || !turnstileHostnameAllowed(v.hostname, result.Hostname) {
		return ErrTurnstileInvalid
	}
	return nil
}

func turnstileHostnameAllowed(allowedHostnames, actualHostname string) bool {
	actualHostname = strings.ToLower(strings.TrimSpace(actualHostname))
	if actualHostname == "" {
		return false
	}
	for _, allowedHostname := range strings.Split(allowedHostnames, ",") {
		if strings.EqualFold(strings.TrimSpace(allowedHostname), actualHostname) {
			return true
		}
	}
	return false
}
