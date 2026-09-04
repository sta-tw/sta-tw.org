package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) (*Handler, error) {
	if service == nil {
		return nil, ErrNotConfigured
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("GET /api/v1/auth/me", h.me)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	mux.HandleFunc("POST /api/v1/auth/email-verification/resend", h.resendEmailVerification)
	mux.HandleFunc("POST /api/v1/auth/email-verification/confirm", h.confirmEmailVerification)
	mux.HandleFunc("POST /api/v1/auth/password-reset/request", h.requestPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/password-reset/confirm", h.confirmPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/password/change", h.changePassword)
	mux.HandleFunc("GET /api/v1/auth/sessions", h.listSessions)
	mux.HandleFunc("DELETE /api/v1/auth/sessions/{sessionID}", h.revokeSession)
	mux.HandleFunc("DELETE /api/v1/auth/sessions", h.revokeOtherSessions)
	mux.HandleFunc("GET /api/v1/auth/admin-mfa/status", h.adminMFAStatus)
	mux.HandleFunc("POST /api/v1/auth/admin-mfa/setup", h.adminMFASetup)
	mux.HandleFunc("POST /api/v1/auth/admin-mfa/enable", h.adminMFAEnable)
	mux.HandleFunc("POST /api/v1/auth/admin-mfa/verify", h.adminMFAVerify)
	mux.HandleFunc("POST /api/v1/auth/admin-mfa/disable", h.adminMFADisable)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/start", h.oauthLoginStart)
	mux.HandleFunc("POST /api/v1/auth/oauth/{provider}/bind/start", h.oauthBindStart)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", h.oauthCallback)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var input RegisterInput
	if err := decodeJSON(r, &input); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	account, err := h.service.Register(r.Context(), input, r)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	response := map[string]any{"account": account}
	if h.service.EmailVerificationConfigured() {
		if _, err := h.service.IssueEmailVerification(r.Context(), account.ID); err != nil {
			h.logger.WarnContext(r.Context(), "queue account email verification failed", "account_id", account.ID)
			response["email_verification_queued"] = false
		} else {
			response["email_verification_queued"] = true
		}
	}
	writeAuthJSON(w, http.StatusCreated, response)
}

func (h *Handler) resendEmailVerification(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.service.AuthorizeMutation(r, session); err != nil {
		writeAuthError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	expiresAt, err := h.service.IssueEmailVerification(r.Context(), session.Session.Account.ID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusAccepted, map[string]any{"email_verification_queued": true, "expires_at": expiresAt})
}

func (h *Handler) confirmEmailVerification(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "verification token is invalid")
		return
	}
	if session, authErr := h.service.Authenticate(r.Context(), r); authErr == nil {
		if err := h.service.AuthorizeMutation(r, session); err != nil {
			writeAuthError(w, http.StatusForbidden, "csrf_required", "request verification failed")
			return
		}
		if err := h.service.ConfirmEmailVerification(r.Context(), session.Session.Account.ID, input.Token); err != nil {
			h.writeServiceError(w, err)
			return
		}
	} else if err := h.service.ConfirmEmailVerificationToken(r.Context(), input.Token); err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusNoContent, nil)
}

func (h *Handler) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	if err := h.service.RequestPasswordReset(r.Context(), input.Email, r); err != nil && !errors.Is(err, ErrNotConfigured) {
		// Never tell the caller whether the address matched; only log real
		// failures and still return 202.
		h.logger.WarnContext(r.Context(), "password reset request failed")
	}
	writeAuthJSON(w, http.StatusAccepted, map[string]any{"status": "accepted"})
}

func (h *Handler) confirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	if err := h.service.ConfirmPasswordReset(r.Context(), input.Token, input.NewPassword); err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusNoContent, nil)
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.service.AuthorizeMutation(r, session); err != nil {
		writeAuthError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	if err := h.service.ChangePassword(r.Context(), session, input.CurrentPassword, input.NewPassword); err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusNoContent, nil)
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	items, err := h.service.ListSessions(r.Context(), session)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) revokeSession(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.service.AuthorizeMutation(r, session); err != nil {
		writeAuthError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	sessionID, err := uuid.Parse(r.PathValue("sessionID"))
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "session id is invalid")
		return
	}
	if err := h.service.RevokeSession(r.Context(), session, sessionID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusNoContent, nil)
}

func (h *Handler) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.service.AuthorizeMutation(r, session); err != nil {
		writeAuthError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	revoked, err := h.service.RevokeOtherSessions(r.Context(), session)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{"revoked": revoked})
}

func (h *Handler) adminMFAStatus(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	enabled, err := h.service.AdminMFAStatus(r.Context(), session.Session.Account.ID)
	if err != nil {
		h.writeMFAServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "required": h.service.AdminMFARequired()})
}

func (h *Handler) adminMFASetup(w http.ResponseWriter, r *http.Request) {
	session, err := h.authenticateMutation(r)
	if err != nil {
		h.writeMFAServiceError(w, err)
		return
	}
	setup, err := h.service.BeginAdminMFA(r.Context(), session.Session.Account.ID, session.Session.Account.Username)
	if err != nil {
		h.writeMFAServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{
		"secret":      setup.Secret,
		"otpauth_url": setup.OTPAuthURL,
		"expires_at":  setup.ExpiresAt,
		"next_step":   "send the current code to POST /api/v1/auth/admin-mfa/enable",
	})
}

func (h *Handler) adminMFAEnable(w http.ResponseWriter, r *http.Request) {
	session, err := h.authenticateMutation(r)
	if err != nil {
		h.writeMFAServiceError(w, err)
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "MFA code is invalid")
		return
	}
	if err := h.service.EnableAdminMFA(r.Context(), session.Session.Account.ID, input.Code); err != nil {
		h.writeMFAServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{"enabled": true})
}

func (h *Handler) adminMFAVerify(w http.ResponseWriter, r *http.Request) {
	session, err := h.authenticateMutation(r)
	if err != nil {
		h.writeMFAServiceError(w, err)
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "MFA code is invalid")
		return
	}
	expiresAt, err := h.service.VerifyAdminMFA(r.Context(), session.Session.Account.ID, input.Code)
	if err != nil {
		h.writeMFAServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{
		"verified":      true,
		"expires_at":    expiresAt,
		"grant_seconds": int(h.service.AdminMFAGrantTTL().Seconds()),
	})
}

func (h *Handler) adminMFADisable(w http.ResponseWriter, r *http.Request) {
	session, err := h.authenticateMutation(r)
	if err != nil {
		h.writeMFAServiceError(w, err)
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "MFA code is invalid")
		return
	}
	if err := h.service.DisableAdminMFA(r.Context(), session.Session.Account.ID, input.Code); err != nil {
		h.writeMFAServiceError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{"enabled": false})
}

func (h *Handler) authenticateMutation(r *http.Request) (RequestSession, error) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		return RequestSession{}, err
	}
	if err := h.service.AuthorizeMutation(r, session); err != nil {
		return RequestSession{}, ErrCSRF
	}
	return session, nil
}

func (h *Handler) writeMFAServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidSession):
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
	case errors.Is(err, ErrCSRF):
		writeAuthError(w, http.StatusForbidden, "csrf_required", "request verification failed")
	case errors.Is(err, ErrAdminRequired):
		writeAuthError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrAdminMFARateLimited):
		writeAuthError(w, http.StatusTooManyRequests, "rate_limited", "too many MFA attempts; try again later")
	case errors.Is(err, ErrAdminMFARequired), errors.Is(err, ErrAdminMFAInvalid):
		writeAuthError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
	case errors.Is(err, ErrAdminMFAConflict):
		writeAuthError(w, http.StatusConflict, "admin_mfa_already_enabled", "administrator MFA is already enabled")
	case errors.Is(err, ErrNotConfigured):
		writeAuthError(w, http.StatusServiceUnavailable, "mfa_unavailable", "administrator MFA is unavailable")
	default:
		h.logger.Error("administrator MFA request failed", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input LoginInput
	if err := decodeJSON(r, &input); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	result, err := h.service.Login(r.Context(), input, r)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			w.Header().Set("Retry-After", "60")
		}
		h.writeServiceError(w, err)
		return
	}
	h.service.SetSessionCookies(w, result)
	writeAuthJSON(w, http.StatusOK, sessionResponse{Account: result.Account, ExpiresAt: result.ExpiresAt})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	writeAuthJSON(w, http.StatusOK, accountResponse{Account: session.Session.Account})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		h.service.ClearSessionCookies(w)
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.service.AuthorizeMutation(r, session); err != nil {
		writeAuthError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	if err := h.service.Logout(r.Context(), session); err != nil {
		h.logger.ErrorContext(r.Context(), "logout failed", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	h.service.ClearSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) oauthLoginStart(w http.ResponseWriter, r *http.Request) {
	url, err := h.service.OAuthStart(r.Context(), r.PathValue("provider"), nil)
	if err != nil {
		h.writeOAuthError(w, err)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handler) oauthBindStart(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Authenticate(r.Context(), r)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.service.AuthorizeMutation(r, session); err != nil {
		writeAuthError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	accountID := session.Session.Account.ID
	url, err := h.service.OAuthStart(r.Context(), r.PathValue("provider"), &accountID)
	if err != nil {
		h.writeOAuthError(w, err)
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]string{"authorization_url": url})
}

func (h *Handler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	var currentAccountID *uuid.UUID
	if session, err := h.service.Authenticate(r.Context(), r); err == nil {
		accountID := session.Session.Account.ID
		currentAccountID = &accountID
	}
	result, err := h.service.OAuthCallback(
		r.Context(),
		r.PathValue("provider"),
		r.URL.Query().Get("state"),
		r.URL.Query().Get("code"),
		currentAccountID,
		r,
	)
	if err != nil {
		h.writeOAuthError(w, err)
		return
	}
	if result.Session != nil {
		h.service.SetSessionCookies(w, *result.Session)
		writeAuthJSON(w, http.StatusOK, sessionResponse{Account: result.Account, ExpiresAt: result.Session.ExpiresAt})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{"account": result.Account, "bound": result.Bound})
}

func (h *Handler) writeOAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrOAuthProvider), errors.Is(err, ErrNotConfigured):
		writeAuthError(w, http.StatusServiceUnavailable, "oauth_unavailable", "OAuth provider is unavailable")
	case errors.Is(err, ErrInvalidSession):
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
	case errors.Is(err, ErrOAuthNotBound):
		writeAuthError(w, http.StatusForbidden, "oauth_not_bound", "this OAuth account is not bound to an STA account")
	case errors.Is(err, ErrConflict):
		writeAuthError(w, http.StatusConflict, "oauth_conflict", "this OAuth identity is already bound")
	default:
		writeAuthError(w, http.StatusBadRequest, "oauth_failed", "OAuth verification failed")
	}
}

type accountResponse struct {
	Account Account `json:"account"`
}

type sessionResponse struct {
	Account   Account   `json:"account"`
	ExpiresAt time.Time `json:"expires_at"`
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request contains more than one JSON value")
	}
	return nil
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "request data is invalid")
	case errors.Is(err, ErrInvalidCredentials):
		writeAuthError(w, http.StatusUnauthorized, "invalid_credentials", "username or password is incorrect")
	case errors.Is(err, ErrRateLimited):
		writeAuthError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
	case errors.Is(err, ErrRateLimitUnavailable):
		writeAuthError(w, http.StatusServiceUnavailable, "rate_limit_unavailable", "request protection is temporarily unavailable")
	case errors.Is(err, ErrNotConfigured):
		writeAuthError(w, http.StatusServiceUnavailable, "auth_unavailable", "authentication service is unavailable")
	case errors.Is(err, ErrConflict):
		writeAuthError(w, http.StatusConflict, "account_conflict", "account data is already in use")
	case errors.Is(err, ErrExpired):
		writeAuthError(w, http.StatusUnprocessableEntity, "verification_expired", "email verification token is invalid or expired")
	case errors.Is(err, ErrInvalidToken):
		writeAuthError(w, http.StatusUnprocessableEntity, "invalid_token", "the token is invalid or has expired")
	case errors.Is(err, ErrNotFound):
		writeAuthError(w, http.StatusNotFound, "not_found", "the resource was not found")
	default:
		// Keep provider/database details out of the HTTP log; errors can contain
		// URLs, SQL fragments, or provider response data.
		h.logger.Error("authentication request failed")
		writeAuthError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

type authErrorBody struct {
	Error authError `json:"error"`
}

type authError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeAuthJSON(w http.ResponseWriter, status int, payload any) {
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	writeAuthJSON(w, status, authErrorBody{Error: authError{Code: code, Message: message}})
}
