package brochurediscovery

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"sta-backend/internal/auth"
	"sta-backend/internal/storage"
)

type Handler struct {
	authService *auth.Service
	repository  Repository
	agentToken  string
	blobStore   storage.BlobStore
	scanner     storage.Scanner
	dispatcher  ExtractionDispatcher
}

func (h *Handler) ConfigureAgent(token string, blobStore storage.BlobStore, scanner storage.Scanner, dispatcher ExtractionDispatcher) {
	if h == nil {
		return
	}
	h.agentToken = strings.TrimSpace(token)
	h.blobStore = blobStore
	h.scanner = scanner
	h.dispatcher = dispatcher
}

func NewHandler(authService *auth.Service, repository Repository) (*Handler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("brochure discovery handler dependencies are missing")
	}
	return &Handler{authService: authService, repository: repository}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/admissions/brochure-discovery/cycles", h.listCycles)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/cycles", h.createCycle)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/start", h.startCycle)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/close", h.closeCycle)
	mux.HandleFunc("GET /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks", h.list)
	mux.HandleFunc("GET /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/events", h.events)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/claim", h.claim)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/candidate", h.candidate)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/failure", h.failure)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/retry", h.retry)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/review", h.review)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/manual-complete", h.manualComplete)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/no-brochure", h.noBrochure)
	mux.HandleFunc("POST /api/v1/internal/admissions/brochure-discovery/claim", h.agentClaim)
	mux.HandleFunc("POST /api/v1/internal/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/candidate", h.agentCandidate)
	mux.HandleFunc("POST /api/v1/internal/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/failure", h.agentFailure)
	mux.HandleFunc("POST /api/v1/internal/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/no-match", h.agentNoMatch)
}

func (h *Handler) agentNoMatch(w http.ResponseWriter, r *http.Request) {
	if !h.requireAgent(w, r) {
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	item, err := h.repository.ReportNoMatchSystem(r.Context(), year, r.PathValue("schoolCode"))
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) agentClaim(w http.ResponseWriter, r *http.Request) {
	if !h.requireAgent(w, r) {
		return
	}
	item, err := h.repository.ClaimNextSystem(r.Context(), 15*time.Minute)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) agentFailure(w http.ResponseWriter, r *http.Request) {
	if !h.requireAgent(w, r) {
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	var input FailureInput
	if decodeJSON(r, &input) != nil || input.Validate() != nil {
		writeError(w, http.StatusBadRequest, "invalid_failure", "failure data is invalid")
		return
	}
	item, err := h.repository.MarkFailureSystem(r.Context(), year, r.PathValue("schoolCode"), input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) agentCandidate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAgent(w, r) {
		return
	}
	if h.blobStore == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxPortfolioFileBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_multipart", "candidate upload is invalid")
		return
	}
	defer r.MultipartForm.RemoveAll()
	detectedYear, err := strconv.Atoi(strings.TrimSpace(r.FormValue("detected_academic_year")))
	input := CandidateInput{
		DetectedAcademicYear: detectedYear,
		SourceURL:            strings.TrimSpace(r.FormValue("source_url")),
		DocumentURL:          strings.TrimSpace(r.FormValue("document_url")),
		Evidence:             make(map[string]any),
	}
	if confidenceText := strings.TrimSpace(r.FormValue("confidence")); confidenceText != "" {
		confidence, parseErr := strconv.ParseFloat(confidenceText, 64)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_candidate", "candidate confidence is invalid")
			return
		}
		input.Confidence = &confidence
	}
	if err := json.Unmarshal([]byte(r.FormValue("evidence")), &input.Evidence); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_candidate", "candidate evidence is invalid")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required", "candidate PDF is required")
		return
	}
	defer file.Close()
	staged, err := storage.StageUpload(header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil || staged.ContentType != "application/pdf" {
		writeError(w, http.StatusBadRequest, "invalid_file", "candidate must be a valid PDF")
		return
	}
	defer staged.Close()
	input.SHA256 = staged.SHA256Hex
	if input.DetectedAcademicYear != year || input.Validate() != nil {
		writeError(w, http.StatusBadRequest, "invalid_candidate", "candidate year, URL, or evidence is invalid")
		return
	}
	if err := storage.ScanStagedUpload(r.Context(), h.scanner, staged); err != nil {
		if errors.Is(err, storage.ErrMalwareDetected) {
			writeError(w, http.StatusUnprocessableEntity, "malware_detected", "candidate file was rejected")
		} else {
			writeError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
		}
		return
	}
	storageKey := "brochures/discovered/" + strconv.Itoa(year) + "-" + r.PathValue("schoolCode") + "-" + staged.SHA256Hex + ".pdf"
	stagedFile, err := os.Open(staged.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	err = h.blobStore.Put(r.Context(), storageKey, stagedFile, staged.Size, staged.ContentType)
	_ = stagedFile.Close()
	if err != nil {
		writeError(w, http.StatusBadGateway, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	item, oldStorageKey, err := h.repository.StoreCandidateSystem(r.Context(), year, r.PathValue("schoolCode"), input, StoredDocumentInput{
		OriginalFileName: staged.OriginalName, StorageKey: storageKey, MIMEType: staged.ContentType,
		FileSizeBytes: staged.Size, SHA256: staged.SHA256Hex,
	})
	if err != nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		h.writeRepositoryError(w, err)
		return
	}
	if oldStorageKey != "" && oldStorageKey != storageKey {
		_ = h.blobStore.Remove(r.Context(), oldStorageKey)
	}
	response := map[string]any{"data": item}
	status := http.StatusCreated
	if h.dispatcher != nil {
		if err := h.dispatcher.QueueDiscoveredBrochureExtraction(r.Context(), year, r.PathValue("schoolCode"), storageKey, staged.SHA256Hex); err != nil {
			status = http.StatusAccepted
			response["extraction_status"] = "retrying"
		} else {
			response["extraction_status"] = "queued"
		}
	}
	writeJSON(w, status, response)
}

func (h *Handler) requireAgent(w http.ResponseWriter, r *http.Request) bool {
	if h.agentToken == "" {
		writeError(w, http.StatusServiceUnavailable, "agent_unavailable", "brochure discovery agent is not configured")
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	expectedHash := sha256.Sum256([]byte(h.agentToken))
	providedHash := sha256.Sum256([]byte(provided))
	if provided == "" || subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid_agent_token", "agent authentication failed")
		return false
	}
	return true
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, false)
	if !ok {
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	items, err := h.repository.ListEvents(r.Context(), session.Session.Account.ID, year, r.PathValue("schoolCode"))
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, false)
	if !ok {
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	query := Query{Status: strings.TrimSpace(r.URL.Query().Get("status")), Limit: 100}
	if query.Status != "" && !ValidStatus(query.Status) {
		writeError(w, http.StatusBadRequest, "invalid_status", "discovery status is invalid")
		return
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		query.Limit, err = strconv.Atoi(raw)
	}
	if err != nil || query.Limit < 1 || query.Limit > 200 {
		writeError(w, http.StatusBadRequest, "invalid_query", "discovery query is invalid")
		return
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		query.Offset, err = strconv.Atoi(raw)
	}
	if err != nil || query.Offset < 0 || query.Offset > 10000 {
		writeError(w, http.StatusBadRequest, "invalid_query", "discovery query is invalid")
		return
	}
	items, err := h.repository.List(r.Context(), session.Session.Account.ID, year, query)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "meta": map[string]any{"academic_year": year, "count": len(items)}})
}

func (h *Handler) listCycles(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, false)
	if !ok {
		return
	}
	items, err := h.repository.ListCycles(r.Context(), session.Session.Account.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) createCycle(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var input CreateCycleInput
	if decodeJSON(r, &input) != nil || input.Validate() != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	cycle, count, err := h.repository.CreateCycle(r.Context(), session.Session.Account.ID, input.AcademicYear)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": cycle, "tasks_created": count})
}

func (h *Handler) startCycle(w http.ResponseWriter, r *http.Request) { h.changeCycle(w, r, true) }
func (h *Handler) closeCycle(w http.ResponseWriter, r *http.Request) { h.changeCycle(w, r, false) }

func (h *Handler) changeCycle(w http.ResponseWriter, r *http.Request, start bool) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	var cycle Cycle
	if start {
		cycle, err = h.repository.StartCycle(r.Context(), session.Session.Account.ID, year)
	} else {
		cycle, err = h.repository.CloseCycle(r.Context(), session.Session.Account.ID, year)
	}
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": cycle})
}

func (h *Handler) claim(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	item, err := h.repository.ClaimNext(r.Context(), session.Session.Account.ID, 15*time.Minute)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) candidate(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var input CandidateInput
	if decodeJSON(r, &input) != nil || input.Validate() != nil {
		writeError(w, http.StatusBadRequest, "invalid_candidate", "candidate data is invalid")
		return
	}
	year, err := pathYear(r)
	if err != nil || input.DetectedAcademicYear != year {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "candidate year must match the discovery cycle")
		return
	}
	item, err := h.repository.SubmitCandidate(r.Context(), session.Session.Account.ID, year, r.PathValue("schoolCode"), input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) failure(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var input FailureInput
	if decodeJSON(r, &input) != nil || input.Validate() != nil {
		writeError(w, http.StatusBadRequest, "invalid_failure", "failure data is invalid")
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	item, err := h.repository.MarkFailure(r.Context(), session.Session.Account.ID, year, r.PathValue("schoolCode"), input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	item, err := h.repository.Retry(r.Context(), session.Session.Account.ID, year, r.PathValue("schoolCode"))
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) review(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var input ReviewInput
	if decodeJSON(r, &input) != nil || input.Validate() != nil {
		writeError(w, http.StatusBadRequest, "invalid_review", "review data is invalid")
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	item, err := h.repository.Review(r.Context(), session.Session.Account.ID, year, r.PathValue("schoolCode"), input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) manualComplete(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	item, err := h.repository.CompleteManual(r.Context(), session.Session.Account.ID, year, r.PathValue("schoolCode"))
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) noBrochure(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if decodeJSON(r, &input) != nil || strings.TrimSpace(input.Reason) == "" || len([]rune(input.Reason)) > 2000 {
		writeError(w, http.StatusBadRequest, "invalid_reason", "a confirmation reason is required")
		return
	}
	year, err := pathYear(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_academic_year", "academic year is invalid")
		return
	}
	item, err := h.repository.ConfirmNoBrochure(r.Context(), session.Session.Account.ID, year, r.PathValue("schoolCode"), input.Reason)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request, mutation bool) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	admin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !admin {
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if mutation && h.authService.AuthorizeMutation(r, session) != nil {
		writeError(w, http.StatusForbidden, "request_verification_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_discovery", "brochure discovery data is invalid")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "brochure discovery task was not found")
	case errors.Is(err, ErrInvalidStatus):
		writeError(w, http.StatusConflict, "invalid_status", "brochure discovery state cannot perform this action")
	case errors.Is(err, ErrAdminRequired):
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func pathYear(r *http.Request) (int, error) {
	year, err := strconv.Atoi(strings.TrimSpace(r.PathValue("academicYear")))
	if err != nil || year < 100 || year > 999 {
		return 0, ErrInvalid
	}
	return year, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
