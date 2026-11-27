package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sta-backend/internal/email"
)

const oauthRegistrationCookie = "sta_oauth_registration"
const discordGuild = "1349295394768556053"
const discordRole = "1482421093829578753"

type oauthRegistration struct {
	Provider string `json:"p"`
	Subject  string `json:"s"`
	Email    string `json:"e"`
	Expires  int64  `json:"x"`
	Nonce    string `json:"n"`
}
type oauthRegistrationInput struct {
	RegisterInput
	CSRFToken string `json:"csrf_token"`
}
type oauthAccountData struct {
	Provider                                                 string
	SubjectHashes                                            [][]byte
	Username                                                 string
	EmailCiphertext, EmailHash, SchoolCiphertext, SchoolHash []byte
	PasswordHash                                             string
	RoleVerified                                             bool
	ChallengeHash, Payload                                   []byte
	NonceHash                                                []byte
	RegistrationExpires                                      time.Time
	Expires                                                  time.Time
}
type oauthRegistrationStore interface {
	CreateOAuthAccount(context.Context, oauthAccountData) (Account, error)
}

func (s *Service) newOAuthRegistration(provider, subject, address string) (string, error) {
	nonce, err := NewOpaqueToken(32)
	if err != nil {
		return "", err
	}
	value, err := json.Marshal(oauthRegistration{Provider: provider, Subject: subject, Email: address, Expires: s.now().Add(15 * time.Minute).Unix(), Nonce: nonce})
	if err != nil {
		return "", err
	}
	ciphertext, err := s.emailCipher.Seal(string(value))
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}
func (s *Service) openOAuthRegistration(value string) (oauthRegistration, error) {
	var p oauthRegistration
	if len(value) > 4096 {
		return p, ErrInvalidToken
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return p, ErrInvalidToken
	}
	plain, err := s.emailCipher.Open(data)
	if err != nil {
		return p, ErrInvalidToken
	}
	if json.Unmarshal([]byte(plain), &p) != nil || p.Expires <= s.now().Unix() || p.Nonce == "" || p.Subject == "" || (p.Provider != "google" && p.Provider != "discord") {
		return oauthRegistration{}, ErrInvalidToken
	}
	return p, nil
}
func (s *Service) discordRoleVerified(ctx context.Context, subject string) (bool, error) {
	if s.discordOAuthBotToken == "" {
		return false, ErrNotConfigured
	}
	for _, c := range subject {
		if c < '0' || c > '9' {
			return false, ErrOAuthSubject
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/v10/guilds/"+discordGuild+"/members/"+subject, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bot "+s.discordOAuthBotToken)
	resp, err := s.oauthHTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("discord member lookup: status %d", resp.StatusCode)
	}
	var result struct {
		Roles []string `json:"roles"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result); err != nil {
		return false, err
	}
	for _, role := range result.Roles {
		if role == discordRole {
			return true, nil
		}
	}
	return false, nil
}
func (s *Service) ConfigureDiscordOAuthBot(token string) {
	s.discordOAuthBotToken = strings.TrimSpace(token)
}
func (s *Service) registerOAuth(ctx context.Context, p oauthRegistration, input oauthRegistrationInput, request *http.Request) (Account, bool, error) {
	store, ok := s.store.(oauthRegistrationStore)
	if !ok || !s.ready() {
		return Account{}, false, ErrNotConfigured
	}
	allowed, err := s.registerAllowed(ctx, request)
	if err != nil {
		return Account{}, false, err
	}
	if !allowed {
		return Account{}, false, ErrRateLimited
	}
	username, err := normalizeUsername(input.Username)
	if err != nil {
		return Account{}, false, err
	}
	contact, err := normalizeAndValidateEmail(input.Email)
	if err != nil {
		return Account{}, false, err
	}
	school, err := normalizeAndValidateEmail(input.SchoolEmail)
	if err != nil || !isSchoolEmail(school) {
		return Account{}, false, ErrInvalidInput
	}
	allowed, err = s.rateAllowed(ctx, s.registerEmailLimiter, "auth-register-school-email", hex.EncodeToString(s.lookupHasher.Hash(school)), 3, time.Hour)
	if err != nil {
		return Account{}, false, err
	}
	if !allowed {
		return Account{}, false, ErrRateLimited
	}
	verified := false
	if p.Provider == "discord" {
		verified, err = s.discordRoleVerified(ctx, p.Subject)
		if err != nil {
			return Account{}, false, err
		}
	}
	c, err := s.emailCipher.Seal(contact)
	if err != nil {
		return Account{}, false, err
	}
	sc, err := s.emailCipher.Seal(school)
	if err != nil {
		return Account{}, false, err
	}
	password, err := NewOpaqueToken(32)
	if err != nil {
		return Account{}, false, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Account{}, false, err
	}
	data := oauthAccountData{Provider: p.Provider, SubjectHashes: s.lookupHasher.Candidates(p.Subject), Username: username, EmailCiphertext: c, EmailHash: s.lookupHasher.Hash(contact), SchoolCiphertext: sc, SchoolHash: s.lookupHasher.Hash(school), PasswordHash: hash, RoleVerified: verified}
	data.NonceHash = HashOpaqueToken(p.Nonce)
	data.RegistrationExpires = time.Unix(p.Expires, 0)
	if !verified {
		if s.emailNotifier == nil {
			return Account{}, false, ErrNotConfigured
		}
		token, e := NewOpaqueToken(32)
		if e != nil {
			return Account{}, false, e
		}
		data.ChallengeHash = HashOpaqueToken(token)
		data.Expires = s.now().UTC().Add(24 * time.Hour)
		link := s.publicBaseURL + "/reset-password?token=" + url.QueryEscape(token)
		text := "歡迎加入 STA！請在 24 小時內點擊以下連結，設定密碼以啟用帳號：\n" + link
		html := email.AccountActivationEmail(email.AccountActivationEmailData{LogoURL: s.publicBaseURL + "/logo.svg", SetURL: link})
		payload, e := json.Marshal(map[string]string{"subject": "[特殊選才資源網] 帳號啟用信", "text": text, "html": html})
		if e != nil {
			return Account{}, false, e
		}
		data.Payload, e = s.emailCipher.Seal(string(payload))
		if e != nil {
			return Account{}, false, e
		}
	}
	account, err := store.CreateOAuthAccount(ctx, data)
	if err != nil {
		return Account{}, false, err
	}
	return account, !verified, nil
}
func (h *Handler) oauthRegistration(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(oauthRegistrationCookie)
	if err != nil {
		h.writeServiceError(w, ErrInvalidToken)
		return
	}
	p, err := h.service.openOAuthRegistration(cookie.Value)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	csrf := base64.RawURLEncoding.EncodeToString(HashOpaqueToken(p.Nonce))
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		writeAuthJSON(w, http.StatusOK, map[string]string{"provider": p.Provider, "email": p.Email, "csrf_token": csrf})
		return
	}
	var input oauthRegistrationInput
	if decodeJSON(r, &input) != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	if subtle.ConstantTimeCompare([]byte(input.CSRFToken), []byte(csrf)) != 1 {
		h.writeServiceError(w, ErrCSRF)
		return
	}
	if !h.verifyTurnstile(w, r, input.TurnstileToken, "signup") {
		return
	}
	account, required, err := h.service.registerOAuth(r.Context(), p, input, r)
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			writeAuthError(w, http.StatusServiceUnavailable, "oauth_unavailable", "OAuth registration is unavailable")
			return
		}
		h.writeServiceError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthRegistrationCookie, Path: "/api/v1/auth/oauth/", HttpOnly: true, Secure: h.service.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if !required {
		session, err := h.service.createSession(r.Context(), account, r)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		h.service.SetSessionCookiesForRequest(w, session, r)
	}
	writeAuthJSON(w, http.StatusCreated, map[string]any{"verification_required": required})
}
