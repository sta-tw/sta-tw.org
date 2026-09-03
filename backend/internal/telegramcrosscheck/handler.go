package telegramcrosscheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"sta-backend/internal/admissions"
	"sta-backend/internal/auth"
	"sta-backend/internal/results"
)

type Handler struct {
	authService           *auth.Service
	repository            Repository
	service               *Service
	cipher                *auth.FieldCipher
	lookupKey             []byte
	serviceToken          string
	allowTestProvisioning bool
	logger                *slog.Logger
}

func NewHandler(
	authService *auth.Service,
	repository Repository,
	writer WillingnessWriter,
	cipher *auth.FieldCipher,
	lookupKey []byte,
	serviceToken string,
	allowTestProvisioning bool,
) (*Handler, error) {
	if authService == nil || repository == nil || writer == nil || cipher == nil || len(lookupKey) != 32 || strings.TrimSpace(serviceToken) == "" {
		return nil, errors.New("Telegram cross-check handler dependencies are missing")
	}
	service, err := NewService(repository, writer)
	if err != nil {
		return nil, err
	}
	return &Handler{
		authService:           authService,
		repository:            repository,
		service:               service,
		cipher:                cipher,
		lookupKey:             append([]byte(nil), lookupKey...),
		serviceToken:          strings.TrimSpace(serviceToken),
		allowTestProvisioning: allowTestProvisioning,
		logger:                slog.Default(),
	}, nil
}

func (handler *Handler) ConfigureLogger(logger *slog.Logger) {
	if handler != nil && logger != nil {
		handler.logger = logger
	}
}

func (handler *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/telegram-cross-check/status", handler.adminStatus)
	mux.HandleFunc("POST /api/v1/admin/telegram-cross-check/participants/sync", handler.syncParticipants)
	mux.HandleFunc("POST /api/v1/internal/telegram-cross-check/bind", handler.bind)
	mux.HandleFunc("POST /api/v1/internal/telegram-cross-check/disable", handler.disable)
	mux.HandleFunc("GET /api/v1/internal/telegram-cross-check/users/{telegramUserID}/dashboard", handler.dashboard)
	mux.HandleFunc("GET /api/v1/internal/telegram-cross-check/users/{telegramUserID}/history", handler.history)
	mux.HandleFunc("POST /api/v1/internal/telegram-cross-check/respond", handler.respond)
	mux.HandleFunc("POST /api/v1/internal/telegram-cross-check/outbox/claim", handler.claim)
	mux.HandleFunc("POST /api/v1/internal/telegram-cross-check/outbox/{deliveryID}/sent", handler.markSent)
	mux.HandleFunc("POST /api/v1/internal/telegram-cross-check/outbox/{deliveryID}/failed", handler.markFailed)
}

func (handler *Handler) adminStatus(writer http.ResponseWriter, request *http.Request) {
	session, ok := handler.requireAdmin(writer, request)
	if !ok {
		return
	}
	status, err := handler.repository.AdminStatus(request.Context(), session.Session.Account.ID)
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": status})
}

func (handler *Handler) syncParticipants(writer http.ResponseWriter, request *http.Request) {
	if !handler.allowTestProvisioning {
		writeError(writer, http.StatusServiceUnavailable, "test_provisioning_disabled", "Telegram test participant provisioning is disabled")
		return
	}
	session, ok := handler.requireAdminMutation(writer, request)
	if !ok {
		return
	}
	var input ParticipantSyncInput
	if err := decodeJSON(request, &input); err != nil || input.Validate() != nil {
		writeError(writer, http.StatusBadRequest, "invalid_participants", "Telegram test participant data is invalid")
		return
	}
	prepared, err := handler.prepareParticipants(input.Participants)
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	result, err := handler.repository.SyncParticipants(request.Context(), session.Session.Account.ID, strings.TrimSpace(input.Reason), prepared)
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": result,
		"meta": map[string]int{"participant_count": len(result)},
	})
}

func (handler *Handler) prepareParticipants(participants []ParticipantInput) ([]PreparedParticipant, error) {
	prepared := make([]PreparedParticipant, 0, len(participants))
	for _, participant := range participants {
		email := fmt.Sprintf("telegram+%d@sta.invalid", participant.TelegramUserID)
		emailCiphertext, err := handler.cipher.Seal(email)
		if err != nil {
			return nil, err
		}
		emailLookupHash, err := auth.LookupHash(handler.lookupKey, auth.NormalizeEmail(email))
		if err != nil {
			return nil, err
		}
		item := PreparedParticipant{
			TelegramUserID:  participant.TelegramUserID,
			Username:        fmt.Sprintf("tg_test_%d", participant.TelegramUserID),
			EmailCiphertext: emailCiphertext,
			EmailLookupHash: emailLookupHash,
			Assignments:     make([]PreparedAssignment, 0, len(participant.Assignments)),
		}
		for _, assignment := range participant.Assignments {
			identifier, err := admissions.ParseProgramIdentifier(strings.TrimSpace(assignment.ProgramIdentifier))
			if err != nil {
				return nil, ErrInvalidInput
			}
			candidate, err := results.NormalizeCandidateNumber(assignment.CandidateNumber)
			if err != nil {
				return nil, ErrInvalidInput
			}
			candidateCiphertext, err := handler.cipher.Seal(candidate)
			if err != nil {
				return nil, err
			}
			candidateLookupHash, err := auth.LookupHash(handler.lookupKey, candidate)
			if err != nil {
				return nil, err
			}
			item.Assignments = append(item.Assignments, PreparedAssignment{
				Identifier:                identifier,
				CandidateNumberCiphertext: candidateCiphertext,
				CandidateNumberLookupHash: candidateLookupHash,
				CandidateNumberLast4:      results.LastFour(candidate),
			})
		}
		prepared = append(prepared, item)
	}
	return prepared, nil
}

func (handler *Handler) bind(writer http.ResponseWriter, request *http.Request) {
	if !handler.requireServiceToken(writer, request) {
		return
	}
	var input BindInput
	if err := decodeJSON(request, &input); err != nil || input.Validate() != nil {
		writeError(writer, http.StatusBadRequest, "invalid_binding", "Telegram private-chat binding is invalid")
		return
	}
	if err := handler.service.Bind(request.Context(), input); err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeNoContent(writer)
}

func (handler *Handler) disable(writer http.ResponseWriter, request *http.Request) {
	if !handler.requireServiceToken(writer, request) {
		return
	}
	var input struct {
		TelegramUserID int64 `json:"telegram_user_id"`
	}
	if err := decodeJSON(request, &input); err != nil || input.TelegramUserID <= 0 {
		writeError(writer, http.StatusBadRequest, "invalid_user", "Telegram user id is invalid")
		return
	}
	if err := handler.service.Disable(request.Context(), input.TelegramUserID); err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeNoContent(writer)
}

func (handler *Handler) dashboard(writer http.ResponseWriter, request *http.Request) {
	if !handler.requireServiceToken(writer, request) {
		return
	}
	telegramUserID, err := parseTelegramUserID(request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_user", "Telegram user id is invalid")
		return
	}
	dashboard, err := handler.service.Dashboard(request.Context(), telegramUserID)
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": dashboard})
}

func (handler *Handler) history(writer http.ResponseWriter, request *http.Request) {
	if !handler.requireServiceToken(writer, request) {
		return
	}
	telegramUserID, err := parseTelegramUserID(request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_user", "Telegram user id is invalid")
		return
	}
	limit := 20
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			writeError(writer, http.StatusBadRequest, "invalid_limit", "History limit is invalid")
			return
		}
	}
	events, err := handler.service.History(request.Context(), telegramUserID, limit)
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": events})
}

func (handler *Handler) respond(writer http.ResponseWriter, request *http.Request) {
	if !handler.requireServiceToken(writer, request) {
		return
	}
	var input RespondInput
	if err := decodeJSON(request, &input); err != nil || input.Validate() != nil {
		writeError(writer, http.StatusBadRequest, "invalid_response", "Telegram willingness response is invalid")
		return
	}
	result, err := handler.service.Respond(request.Context(), input)
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": result})
}

func (handler *Handler) claim(writer http.ResponseWriter, request *http.Request) {
	if !handler.requireServiceToken(writer, request) {
		return
	}
	var input ClaimInput
	if err := decodeJSON(request, &input); err != nil || input.Normalize() != nil {
		writeError(writer, http.StatusBadRequest, "invalid_claim", "Telegram delivery claim is invalid")
		return
	}
	deliveries, err := handler.repository.ClaimDeliveries(request.Context(), input.Limit)
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": deliveries})
}

func (handler *Handler) markSent(writer http.ResponseWriter, request *http.Request) {
	if !handler.requireServiceToken(writer, request) {
		return
	}
	deliveryID, err := uuid.Parse(strings.TrimSpace(request.PathValue("deliveryID")))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_delivery", "Telegram delivery id is invalid")
		return
	}
	var input SentInput
	if err := decodeJSON(request, &input); err != nil || input.TelegramMessageID <= 0 {
		writeError(writer, http.StatusBadRequest, "invalid_message", "Telegram message id is invalid")
		return
	}
	if err := handler.repository.MarkSent(request.Context(), deliveryID, input.TelegramMessageID); err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeNoContent(writer)
}

func (handler *Handler) markFailed(writer http.ResponseWriter, request *http.Request) {
	if !handler.requireServiceToken(writer, request) {
		return
	}
	deliveryID, err := uuid.Parse(strings.TrimSpace(request.PathValue("deliveryID")))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_delivery", "Telegram delivery id is invalid")
		return
	}
	var input FailedInput
	if err := decodeJSON(request, &input); err != nil || input.Validate() != nil {
		writeError(writer, http.StatusBadRequest, "invalid_failure", "Telegram delivery failure is invalid")
		return
	}
	if err := handler.repository.MarkFailed(request.Context(), deliveryID, input.Error, input.Retryable); err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	writeNoContent(writer)
}

func (handler *Handler) requireServiceToken(writer http.ResponseWriter, request *http.Request) bool {
	const prefix = "Bearer "
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) || !auth.ConstantTimeStringEqual(strings.TrimSpace(strings.TrimPrefix(authorization, prefix)), handler.serviceToken) {
		writeError(writer, http.StatusUnauthorized, "unauthorized", "Telegram service authentication is required")
		return false
	}
	return true
}

func (handler *Handler) requireAdminMutation(writer http.ResponseWriter, request *http.Request) (auth.RequestSession, bool) {
	session, ok := handler.requireAdmin(writer, request)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := handler.authService.AuthorizeMutation(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_required", "Request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (handler *Handler) requireAdmin(writer http.ResponseWriter, request *http.Request) (auth.RequestSession, bool) {
	session, err := handler.authService.Authenticate(request.Context(), request)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeError(writer, http.StatusPreconditionRequired, "admin_mfa_required", "Administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeError(writer, http.StatusUnauthorized, "unauthorized", "Authentication is required")
		return auth.RequestSession{}, false
	}
	isAdmin, err := handler.repository.IsAdmin(request.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "internal_error", "Internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeError(writer, http.StatusForbidden, "admin_required", "Administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := handler.authService.RequireAdminMFA(request.Context(), session.Session.Account.ID, request.Header.Get("X-MFA-Code")); err != nil {
		writeError(writer, http.StatusPreconditionRequired, "admin_mfa_required", "Administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (handler *Handler) writeServiceError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, results.ErrInvalidInput), errors.Is(err, results.ErrInvalidWillingness):
		writeError(writer, http.StatusBadRequest, "invalid_request", "Telegram cross-check request is invalid")
	case errors.Is(err, ErrNotFound), errors.Is(err, results.ErrNotFound):
		writeError(writer, http.StatusNotFound, "not_found", "Telegram cross-check resource was not found")
	case errors.Is(err, ErrConflict), errors.Is(err, results.ErrConflict):
		writeError(writer, http.StatusConflict, "conflict", "Telegram cross-check resource conflicts with existing data")
	case errors.Is(err, ErrAdminRequired):
		writeError(writer, http.StatusForbidden, "admin_required", "Administrator permission is required")
	case errors.Is(err, ErrProvisioningDisabled):
		writeError(writer, http.StatusServiceUnavailable, "test_provisioning_disabled", "Telegram test participant provisioning is disabled")
	case errors.Is(err, ErrInvalidState):
		writeError(writer, http.StatusConflict, "invalid_state", "Telegram delivery is no longer in the requested state")
	default:
		handler.logger.Error("Telegram cross-check request failed", "error", err)
		writeError(writer, http.StatusInternalServerError, "internal_error", "Internal server error")
	}
}

func parseTelegramUserID(request *http.Request) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(request.PathValue("telegramUserID")), 10, 64)
	if err != nil || value <= 0 {
		return 0, ErrInvalidInput
	}
	return value, nil
}

func decodeJSON(request *http.Request, destination any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

type errorBody struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, errorBody{Error: apiError{Code: code, Message: message}})
}

func writeNoContent(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusNoContent)
}
