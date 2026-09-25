package admissions

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/security"
	"sta-backend/internal/storage"
)

type BrochureHandler struct {
	authService   *auth.Service
	repository    BrochureRepository
	blobStore     storage.BlobStore
	scanner       storage.Scanner
	dispatcher    BrochureExtractionDispatcher
	uploadLimiter *security.FixedWindowLimiter
}

func NewBrochureHandler(authService *auth.Service, repository BrochureRepository, blobStore storage.BlobStore) (*BrochureHandler, error) {
	return NewBrochureHandlerWithDispatcher(authService, repository, blobStore, nil)
}

func NewBrochureHandlerWithDispatcher(authService *auth.Service, repository BrochureRepository, blobStore storage.BlobStore, dispatcher BrochureExtractionDispatcher) (*BrochureHandler, error) {
	return NewBrochureHandlerWithDispatcherAndScanner(authService, repository, blobStore, dispatcher, nil)
}

func NewBrochureHandlerWithDispatcherAndScanner(authService *auth.Service, repository BrochureRepository, blobStore storage.BlobStore, dispatcher BrochureExtractionDispatcher, scanner storage.Scanner) (*BrochureHandler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("brochure handler dependencies are missing")
	}
	return &BrochureHandler{
		authService:   authService,
		repository:    repository,
		blobStore:     blobStore,
		scanner:       scanner,
		dispatcher:    dispatcher,
		uploadLimiter: security.NewFixedWindowLimiter(30, time.Minute, 4096),
	}, nil
}

func (h *BrochureHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admissions/brochures/{academicYear}/{schoolCode}/download", h.downloadPublic)
	mux.HandleFunc("GET /api/v1/admissions/brochures/{schoolCode}", h.listPublicBySchool)
	mux.HandleFunc("GET /api/v1/admin/admissions/brochures", h.list)
	mux.HandleFunc("GET /api/v1/admin/admissions/brochures/{academicYear}/{schoolCode}/events", h.listEvents)
	mux.HandleFunc("GET /api/v1/admin/admissions/brochures/{academicYear}/{schoolCode}/download", h.downloadAdmin)
	mux.HandleFunc("GET /api/v1/admin/admissions/brochures/{academicYear}/{schoolCode}/content", h.contentAdmin)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochures", h.upload)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochures/direct", h.uploadDirect)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochures/{academicYear}/{schoolCode}/review", h.review)
	mux.HandleFunc("POST /api/v1/admin/admissions/brochures/{academicYear}/{schoolCode}/visibility", h.visibility)
}

func (h *BrochureHandler) downloadPublic(w http.ResponseWriter, r *http.Request) {
	if h.blobStore == nil {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	year, schoolCode, err := parseBrochurePath(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_brochure_path", "brochure path is invalid")
		return
	}
	document, err := h.repository.GetPublishedBrochure(r.Context(), year, schoolCode)
	if errors.Is(err, ErrNotFound) {
		writeAdmissionError(w, http.StatusNotFound, "not_found", "published brochure was not found")
		return
	}
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	h.writeDownload(w, r, document)
}

// listPublicBySchool lists every published academic_year's brochure for a school (歷史簡章).
func (h *BrochureHandler) listPublicBySchool(w http.ResponseWriter, r *http.Request) {
	schoolCode := r.PathValue("schoolCode")
	if !validSchoolCode(schoolCode) {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_school_code", "school code is invalid")
		return
	}
	documents, err := h.repository.ListPublishedBrochures(r.Context(), schoolCode)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": documents})
}

func (h *BrochureHandler) downloadAdmin(w http.ResponseWriter, r *http.Request) {
	if h.blobStore == nil {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	year, schoolCode, err := parseBrochurePath(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_brochure_path", "brochure path is invalid")
		return
	}
	document, err := h.repository.GetBrochure(r.Context(), session.Session.Account.ID, year, schoolCode)
	if errors.Is(err, ErrNotFound) {
		writeAdmissionError(w, http.StatusNotFound, "not_found", "brochure was not found")
		return
	}
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.writeDownload(w, r, document)
}

// contentAdmin proxies a published brochure's PDF bytes instead of a presigned
// URL, since the object-storage host isn't always browser-resolvable.
func (h *BrochureHandler) contentAdmin(w http.ResponseWriter, r *http.Request) {
	if h.blobStore == nil {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	year, schoolCode, err := parseBrochurePath(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_brochure_path", "brochure path is invalid")
		return
	}
	document, err := h.repository.GetBrochure(r.Context(), session.Session.Account.ID, year, schoolCode)
	if errors.Is(err, ErrNotFound) {
		writeAdmissionError(w, http.StatusNotFound, "not_found", "brochure was not found")
		return
	}
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	reader, ok := h.blobStore.(storage.BlobReader)
	if !ok {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "brochure content proxy is unavailable")
		return
	}
	object, err := reader.Get(r.Context(), brochureStorageKey(document))
	if err != nil {
		writeAdmissionError(w, http.StatusBadGateway, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	defer object.Close()
	contentType := object.ContentType
	if contentType == "" {
		contentType = "application/pdf"
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `inline; filename="brochure.pdf"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if object.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	}
	if _, err := io.Copy(w, object); err != nil {
		return
	}
}

func (h *BrochureHandler) writeDownload(w http.ResponseWriter, r *http.Request, document BrochureDocument) {
	url, err := h.blobStore.PresignGet(r.Context(), brochureStorageKey(document), 5*time.Minute)
	if err != nil {
		writeAdmissionError(w, http.StatusBadGateway, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": document, "url": url.String(), "expires_in": 300})
}

func (h *BrochureHandler) list(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	year, err := parseOptionalBrochureYear(r.URL.Query().Get("academic_year"))
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_academic_year", "academic_year is invalid")
		return
	}
	items, err := h.repository.ListBrochures(r.Context(), session.Session.Account.ID, year)
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *BrochureHandler) listEvents(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	year, schoolCode, err := parseBrochurePath(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_brochure_path", "brochure path is invalid")
		return
	}
	items, err := h.repository.ListBrochureEvents(r.Context(), session.Session.Account.ID, year, schoolCode)
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *BrochureHandler) upload(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	if h.blobStore == nil {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	now := time.Now()
	rl := h.uploadLimiter.Take(session.Session.Account.ID.String(), now)
	security.WriteRateLimitHeaders(w, rl, now)
	if !rl.Allowed {
		writeAdmissionError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxPortfolioFileBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_multipart", "brochure upload is invalid")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "file_required", "brochure PDF is required")
		return
	}
	defer file.Close()
	staged, err := storage.StageUpload(header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil || staged.ContentType != "application/pdf" {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_file", "brochure must be a valid PDF")
		return
	}
	defer staged.Close()
	if err := storage.ScanStagedUpload(r.Context(), h.scanner, staged); err != nil {
		if errors.Is(err, storage.ErrMalwareDetected) {
			writeAdmissionError(w, http.StatusUnprocessableEntity, "malware_detected", "uploaded file was rejected")
		} else {
			writeAdmissionError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
		}
		return
	}
	// Identity isn't required at upload time; the worker detects it and the admin confirms on review.
	uploadID := uuid.New()
	storageKey := "brochures/uploads/" + uploadID.String() + ".pdf"
	stagedFile, err := os.Open(staged.Path)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	err = h.blobStore.Put(r.Context(), storageKey, stagedFile, staged.Size, staged.ContentType)
	_ = stagedFile.Close()
	if err != nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		writeAdmissionError(w, http.StatusBadGateway, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	jobDispatcher, ok := h.dispatcher.(BrochureUploadExtractionDispatcher)
	if !ok {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		writeAdmissionError(w, http.StatusServiceUnavailable, "extraction_unavailable", "brochure extraction service is unavailable")
		return
	}
	jobID, dispatchErr := jobDispatcher.QueueUploadedBrochureExtraction(r.Context(), session.Session.Account.ID, BrochureDocumentInput{
		OriginalFileName: staged.OriginalName,
		StorageKey:       storageKey,
		MIMEType:         staged.ContentType,
		FileSizeBytes:    staged.Size,
		SHA256:           staged.SHA256Hex,
	})
	if jobID == uuid.Nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		if dispatchErr != nil {
			h.writeAdminError(w, dispatchErr)
		} else {
			writeAdmissionError(w, http.StatusServiceUnavailable, "extraction_unavailable", "brochure extraction service is unavailable")
		}
		return
	}
	responseStatus := http.StatusCreated
	response := map[string]any{
		"data": map[string]any{
			"upload_id":          jobID,
			"job_id":             jobID,
			"original_file_name": staged.OriginalName,
			"mime_type":          staged.ContentType,
			"file_size_bytes":    staged.Size,
			"sha256":             staged.SHA256Hex,
			"status":             "queued",
		},
		"job_id": jobID,
	}
	if dispatchErr != nil {
		// Broker outage: keep the stored PDF, let the admin retry extraction later.
		var retryable interface{ Retryable() bool }
		if errors.As(dispatchErr, &retryable) && retryable.Retryable() {
			responseStatus = http.StatusAccepted
			response["extraction_status"] = "retrying"
		} else {
			_ = h.blobStore.Remove(r.Context(), storageKey)
			h.writeAdminError(w, dispatchErr)
			return
		}
	} else {
		response["extraction_status"] = "queued"
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, responseStatus, response)
}

// uploadDirect stages/scans/stores like upload(), but skips extraction and takes
// an admin-declared academic_year/school_code directly; still lands as pending.
func (h *BrochureHandler) uploadDirect(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	if h.blobStore == nil {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	now := time.Now()
	rl := h.uploadLimiter.Take(session.Session.Account.ID.String(), now)
	security.WriteRateLimitHeaders(w, rl, now)
	if !rl.Allowed {
		writeAdmissionError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxPortfolioFileBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_multipart", "brochure upload is invalid")
		return
	}
	defer r.MultipartForm.RemoveAll()
	academicYear, err := strconv.Atoi(strings.TrimSpace(r.FormValue("academic_year")))
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_academic_year", "academic_year must be a 3-digit number")
		return
	}
	schoolCode := strings.TrimSpace(r.FormValue("school_code"))
	sourceURL := strings.TrimSpace(r.FormValue("source_url"))
	file, header, err := r.FormFile("file")
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "file_required", "brochure PDF is required")
		return
	}
	defer file.Close()
	staged, err := storage.StageUpload(header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil || staged.ContentType != "application/pdf" {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_file", "brochure must be a valid PDF")
		return
	}
	defer staged.Close()
	if err := storage.ScanStagedUpload(r.Context(), h.scanner, staged); err != nil {
		if errors.Is(err, storage.ErrMalwareDetected) {
			writeAdmissionError(w, http.StatusUnprocessableEntity, "malware_detected", "uploaded file was rejected")
		} else {
			writeAdmissionError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
		}
		return
	}
	storageKey := "brochures/uploads/" + uuid.New().String() + ".pdf"
	stagedFile, err := os.Open(staged.Path)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	err = h.blobStore.Put(r.Context(), storageKey, stagedFile, staged.Size, staged.ContentType)
	_ = stagedFile.Close()
	if err != nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		writeAdmissionError(w, http.StatusBadGateway, "storage_unavailable", "brochure storage is unavailable")
		return
	}
	document, _, err := h.repository.CreateBrochure(r.Context(), session.Session.Account.ID, BrochureDocumentInput{
		AcademicYear:     academicYear,
		SchoolCode:       schoolCode,
		OriginalFileName: staged.OriginalName,
		StorageKey:       storageKey,
		MIMEType:         staged.ContentType,
		FileSizeBytes:    staged.Size,
		SHA256:           staged.SHA256Hex,
		SourceURL:        sourceURL,
	})
	if err != nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		if errors.Is(err, ErrInvalidProgram) {
			writeAdmissionError(w, http.StatusBadRequest, "invalid_input", "academic_year, school_code or source_url is invalid")
			return
		}
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusCreated, map[string]any{"data": document})
}

func (h *BrochureHandler) review(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	year, schoolCode, err := parseBrochurePath(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_brochure_path", "brochure path is invalid")
		return
	}
	var input struct {
		Approved bool   `json:"approved"`
		Reason   string `json:"reason"`
	}
	if err := decodeBrochureJSON(r, &input); err != nil || (!input.Approved && strings.TrimSpace(input.Reason) == "") || len(input.Reason) > 2000 {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_review", "brochure review is invalid")
		return
	}
	document, err := h.repository.ReviewBrochure(r.Context(), session.Session.Account.ID, year, schoolCode, input.Approved, strings.TrimSpace(input.Reason))
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": document})
}

func (h *BrochureHandler) visibility(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	year, schoolCode, err := parseBrochurePath(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_brochure_path", "brochure path is invalid")
		return
	}
	var input struct {
		Published bool   `json:"published"`
		Reason    string `json:"reason"`
	}
	if err := decodeBrochureJSON(r, &input); err != nil || (!input.Published && strings.TrimSpace(input.Reason) == "") || len(input.Reason) > 2000 {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_visibility", "brochure visibility is invalid")
		return
	}
	document, err := h.repository.SetBrochurePublished(r.Context(), session.Session.Account.ID, year, schoolCode, input.Published, strings.TrimSpace(input.Reason))
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": document})
}

func (h *BrochureHandler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeAdmissionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeAdmissionError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	if ok, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID); err != nil || !ok {
		writeAdmissionError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeAdmissionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *BrochureHandler) requireAdminMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeAdmissionError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeAdmissionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *BrochureHandler) writeAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeAdmissionError(w, http.StatusNotFound, "not_found", "brochure was not found")
	case errors.Is(err, ErrInvalidProgram):
		writeAdmissionError(w, http.StatusConflict, "invalid_status", "brochure state is invalid")
	default:
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseBrochurePath(r *http.Request) (int, string, error) {
	year, err := strconv.Atoi(r.PathValue("academicYear"))
	schoolCode := r.PathValue("schoolCode")
	if err != nil || year < 100 || year > 999 || !validSchoolCode(schoolCode) {
		return 0, "", ErrInvalidProgram
	}
	return year, schoolCode, nil
}

func parseOptionalBrochureYear(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	year, err := strconv.Atoi(raw)
	if err != nil || year < 100 || year > 999 {
		return 0, ErrInvalidProgram
	}
	return year, nil
}

func decodeBrochureJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return err
	}
	return nil
}

func brochureStorageKey(document BrochureDocument) string {
	return document.storageKey
}
