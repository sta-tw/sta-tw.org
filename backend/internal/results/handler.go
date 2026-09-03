package results

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
)

type Handler struct {
	authService *auth.Service
	repository  Repository
	cipher      *auth.FieldCipher
	lookupKey   []byte
}

type ImportHandler struct {
	authService *auth.Service
	importer    Importer
}

func NewHandler(authService *auth.Service, repository Repository, cipher *auth.FieldCipher, lookupKey []byte) (*Handler, error) {
	if authService == nil || repository == nil || cipher == nil || len(lookupKey) != 32 {
		return nil, errors.New("result handler dependencies are missing")
	}
	return &Handler{authService: authService, repository: repository, cipher: cipher, lookupKey: append([]byte(nil), lookupKey...)}, nil
}

func NewImportHandler(authService *auth.Service, importer Importer) (*ImportHandler, error) {
	if authService == nil || importer == nil {
		return nil, errors.New("result import dependencies are missing")
	}
	return &ImportHandler{authService: authService, importer: importer}, nil
}

func (h *ImportHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/results/batches", h.listBatches)
	mux.HandleFunc("GET /api/v1/admin/results/batches/{batchID}", h.getBatch)
	mux.HandleFunc("POST /api/v1/admin/results/import", h.importBatch)
	mux.HandleFunc("POST /api/v1/admin/results/{batchID}/publish", h.publishBatch)
	mux.HandleFunc("POST /api/v1/admin/results/{batchID}/inquiries/acceptance-deadline", h.createAcceptanceDeadlineInquiries)
	mux.HandleFunc("POST /api/v1/admin/results/{resultID}/correct", h.correctResult)
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/applications/{applicationID}/result", h.getReport)
	mux.HandleFunc("GET /api/v1/applications/{applicationID}/inquiries", h.listInquiries)
	mux.HandleFunc("PUT /api/v1/applications/{applicationID}/candidate-number", h.setCandidateNumber)
	mux.HandleFunc("PUT /api/v1/applications/{applicationID}/willingness", h.setWillingness)
}

func (h *ImportHandler) importBatch(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	var input ImportBatchInput
	if err := decodeResultJSON(r, &input); err != nil || input.Validate() != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_import", "result import is invalid")
		return
	}
	for _, row := range input.Rows {
		if err := row.Validate(); err != nil {
			writeResultError(w, http.StatusBadRequest, "invalid_import_row", "result import row is invalid")
			return
		}
	}
	batchID, err := h.importer.ImportOfficialBatch(r.Context(), session.Session.Account.ID, input)
	if err != nil {
		h.writeImporterError(w, err)
		return
	}
	writeResultJSON(w, http.StatusCreated, map[string]any{"batch_id": batchID})
}

func (h *ImportHandler) publishBatch(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	batchID, err := uuid.Parse(r.PathValue("batchID"))
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_batch_id", "batch id is invalid")
		return
	}
	if err := h.importer.PublishOfficialBatch(r.Context(), session.Session.Account.ID, batchID); err != nil {
		h.writeImporterError(w, err)
		return
	}
	writeResultNoContent(w)
}

func (h *ImportHandler) createAcceptanceDeadlineInquiries(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	batchID, err := uuid.Parse(r.PathValue("batchID"))
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_batch_id", "batch id is invalid")
		return
	}
	var input struct {
		Deadline time.Time `json:"deadline"`
	}
	if err := decodeResultJSON(r, &input); err != nil || input.Deadline.IsZero() || input.Deadline.Before(time.Now().UTC()) {
		writeResultError(w, http.StatusBadRequest, "invalid_deadline", "deadline is invalid")
		return
	}
	if err := h.importer.CreateAcceptanceDeadlineInquiries(r.Context(), session.Session.Account.ID, batchID, input.Deadline.UTC()); err != nil {
		h.writeImporterError(w, err)
		return
	}
	writeResultNoContent(w)
}

func (h *ImportHandler) correctResult(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	resultID, err := uuid.Parse(r.PathValue("resultID"))
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_result_id", "result id is invalid")
		return
	}
	var input OfficialResultCorrectionInput
	if err := decodeResultJSON(r, &input); err != nil || input.Validate() != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_correction", "correction data is invalid")
		return
	}
	if err := h.importer.CorrectOfficialResult(r.Context(), session.Session.Account.ID, resultID, input); err != nil {
		h.writeImporterError(w, err)
		return
	}
	writeResultNoContent(w)
}

func (h *ImportHandler) listBatches(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	query, err := parseAdminBatchQuery(r)
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_query", "result batch query is invalid")
		return
	}
	batches, err := h.importer.ListAdminBatches(r.Context(), session.Session.Account.ID, query)
	if err != nil {
		h.writeImporterError(w, err)
		return
	}
	writeResultJSON(w, http.StatusOK, map[string]any{
		"data": batches,
		"meta": map[string]any{"limit": query.Limit, "offset": query.Offset, "count": len(batches)},
	})
}

func (h *ImportHandler) getBatch(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	batchID, err := uuid.Parse(r.PathValue("batchID"))
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_batch_id", "batch id is invalid")
		return
	}
	detail, err := h.importer.GetAdminBatch(r.Context(), session.Session.Account.ID, batchID)
	if err != nil {
		h.writeImporterError(w, err)
		return
	}
	writeResultJSON(w, http.StatusOK, map[string]any{"data": detail})
}

func (h *ImportHandler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeResultError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeResultError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.importer.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeResultError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeResultError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeResultError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *ImportHandler) requireAdminMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeResultError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeResultError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) getReport(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r, false)
	if !ok {
		return
	}
	applicationID, err := parseApplicationID(r)
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_application_id", "application id is invalid")
		return
	}
	report, err := h.repository.GetReport(r.Context(), session.Session.Account.ID, applicationID)
	if errors.Is(err, ErrNotFound) {
		writeResultError(w, http.StatusNotFound, "result_not_found", "official result is not available")
		return
	}
	if err != nil {
		writeResultError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeResultJSON(w, http.StatusOK, map[string]any{"data": report})
}

func (h *Handler) setCandidateNumber(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r, true)
	if !ok {
		return
	}
	applicationID, err := parseApplicationID(r)
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_application_id", "application id is invalid")
		return
	}
	var input CandidateNumberInput
	if err := decodeResultJSON(r, &input); err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_request", "candidate number is invalid")
		return
	}
	candidateNumber, err := NormalizeCandidateNumber(input.CandidateNumber)
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_candidate_number", "candidate number is invalid")
		return
	}
	ciphertext, err := h.cipher.Seal(candidateNumber)
	if err != nil {
		writeResultError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	lookupHash, err := auth.LookupHash(h.lookupKey, candidateNumber)
	if err != nil {
		writeResultError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if err := h.repository.SetCandidateNumber(r.Context(), session.Session.Account.ID, applicationID, ciphertext, lookupHash, LastFour(candidateNumber)); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeResultJSON(w, http.StatusNoContent, nil)
}

func (h *Handler) setWillingness(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r, true)
	if !ok {
		return
	}
	applicationID, err := parseApplicationID(r)
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_application_id", "application id is invalid")
		return
	}
	var input WillingnessInput
	if err := decodeResultJSON(r, &input); err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_request", "willingness is invalid")
		return
	}
	if err := ValidateWillingness(input.Value); err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_willingness", err.Error())
		return
	}
	response, err := h.repository.SetWillingness(r.Context(), session.Session.Account.ID, applicationID, input.Value, input.InquiryID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeResultJSON(w, http.StatusOK, map[string]any{"data": response})
}

func (h *Handler) listInquiries(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r, false)
	if !ok {
		return
	}
	applicationID, err := parseApplicationID(r)
	if err != nil {
		writeResultError(w, http.StatusBadRequest, "invalid_application_id", "application id is invalid")
		return
	}
	inquiries, err := h.repository.ListInquiries(r.Context(), session.Session.Account.ID, applicationID)
	if errors.Is(err, ErrNotFound) {
		writeResultError(w, http.StatusNotFound, "inquiries_not_found", "application inquiries are not available")
		return
	}
	if err != nil {
		writeResultError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeResultJSON(w, http.StatusOK, map[string]any{"data": inquiries})
}

func (h *Handler) requireVerified(w http.ResponseWriter, r *http.Request, mutation bool) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		writeResultError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	if session.Session.Account.IdentityStatus != "student" {
		writeResultError(w, http.StatusForbidden, "verification_required", "verified identity is required")
		return auth.RequestSession{}, false
	}
	if mutation {
		if err := h.authService.AuthorizeMutation(r, session); err != nil {
			writeResultError(w, http.StatusForbidden, "csrf_required", "request verification failed")
			return auth.RequestSession{}, false
		}
	}
	return session, true
}

func parseApplicationID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(r.PathValue("applicationID")))
}

func decodeResultJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
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

func (h *Handler) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidWillingness):
		writeResultError(w, http.StatusBadRequest, "invalid_request", "result request is invalid")
	case errors.Is(err, ErrNotFound):
		writeResultError(w, http.StatusNotFound, "not_found", "result resource not found")
	case errors.Is(err, ErrConflict):
		writeResultError(w, http.StatusConflict, "conflict", "result resource conflict")
	default:
		writeResultError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func (h *ImportHandler) writeImporterError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidQuery):
		writeResultError(w, http.StatusBadRequest, "invalid_request", "result request is invalid")
	case errors.Is(err, ErrNotFound):
		writeResultError(w, http.StatusNotFound, "not_found", "result resource was not found")
	case errors.Is(err, ErrConflict):
		writeResultError(w, http.StatusConflict, "conflict", "result resource already exists")
	case errors.Is(err, ErrAdminRequired):
		writeResultError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	default:
		writeResultError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

type resultErrorBody struct {
	Error resultError `json:"error"`
}

type resultError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeResultJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeResultError(w http.ResponseWriter, status int, code, message string) {
	writeResultJSON(w, status, resultErrorBody{Error: resultError{Code: code, Message: message}})
}

func writeResultNoContent(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func parseAdminBatchQuery(r *http.Request) (AdminResultBatchQuery, error) {
	values := r.URL.Query()
	query := AdminResultBatchQuery{SchoolCode: strings.TrimSpace(values.Get("school_code")), Status: strings.TrimSpace(values.Get("status")), Limit: 50}
	if raw := strings.TrimSpace(values.Get("academic_year")); raw != "" {
		year, err := strconv.Atoi(raw)
		if err != nil {
			return AdminResultBatchQuery{}, ErrInvalidQuery
		}
		query.AcademicYear = year
	}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return AdminResultBatchQuery{}, ErrInvalidQuery
		}
		query.Limit = limit
	}
	if raw := strings.TrimSpace(values.Get("offset")); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil {
			return AdminResultBatchQuery{}, ErrInvalidQuery
		}
		query.Offset = offset
	}
	if err := query.Validate(); err != nil {
		return AdminResultBatchQuery{}, err
	}
	return query, nil
}
