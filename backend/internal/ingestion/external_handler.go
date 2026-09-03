package ingestion

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/admissions"
	"sta-backend/internal/auth"
	"sta-backend/internal/jobs"
	"sta-backend/internal/results"
	"sta-backend/internal/storage"
)

// ExternalHandler exposes the only service-token surface used by external
// agents and the Python extractor. It accepts sources and extraction results,
// but it never grants access to admin review or publication operations.
type ExternalHandler struct {
	authService        *auth.Service
	repository         *PostgresRepository
	brochureRepository admissions.SystemBrochureRepository
	resultApplier      CandidateListResultApplier
	service            *Service
	blobStore          storage.BlobStore
	scanner            storage.Scanner
	serviceToken       string
}

func NewExternalHandler(
	authService *auth.Service,
	repository *PostgresRepository,
	brochureRepository admissions.SystemBrochureRepository,
	resultApplier CandidateListResultApplier,
	service *Service,
	blobStore storage.BlobStore,
	scanner storage.Scanner,
	serviceToken string,
) (*ExternalHandler, error) {
	if authService == nil || repository == nil || brochureRepository == nil || resultApplier == nil || service == nil {
		return nil, errors.New("external extraction handler dependencies are missing")
	}
	return &ExternalHandler{
		authService: authService, repository: repository, brochureRepository: brochureRepository,
		resultApplier: resultApplier, service: service, blobStore: blobStore, scanner: scanner,
		serviceToken: strings.TrimSpace(serviceToken),
	}, nil
}

func (h *ExternalHandler) RegisterRoutes(mux *http.ServeMux) {
	// Admins can upload a list without needing to first convert it to JSON.
	mux.HandleFunc("POST /api/v1/admin/extraction/candidate-lists", h.adminCandidateList)

	// The service-token API is used by external search clients and by the
	// Python extraction process. All writes remain pending until review.
	mux.HandleFunc("POST /api/v1/internal/extraction/brochures", h.externalBrochure)
	mux.HandleFunc("POST /api/v1/internal/extraction/candidate-lists", h.externalCandidateList)
	mux.HandleFunc("POST /api/v1/internal/extraction/jobs/claim", h.claim)
	mux.HandleFunc("GET /api/v1/internal/extraction/jobs/{jobID}", h.jobStatus)
	mux.HandleFunc("POST /api/v1/internal/extraction/jobs/{jobID}/result", h.result)
	mux.HandleFunc("POST /api/v1/internal/extraction/jobs/{jobID}/failure", h.failure)
}

func (h *ExternalHandler) adminCandidateList(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	h.uploadCandidateList(w, r, &session.Session.Account.ID)
}

func (h *ExternalHandler) externalCandidateList(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w, r) {
		return
	}
	h.uploadCandidateList(w, r, nil)
}

func (h *ExternalHandler) externalBrochure(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w, r) {
		return
	}
	if h.blobStore == nil {
		writeIngestionError(w, http.StatusServiceUnavailable, "storage_unavailable", "extraction storage is unavailable")
		return
	}
	upload, year, schoolCode, sourceURL, _, err := h.parseFileUpload(w, r, jobs.SourceTypeBrochure)
	if err != nil {
		return
	}
	if upload.ContentType != "application/pdf" {
		upload.Close()
		writeIngestionError(w, http.StatusBadRequest, "invalid_file", "brochure must be a valid PDF")
		return
	}
	defer upload.Close()

	storageKey := fmt.Sprintf("brochures/external/%03d-%s-%s.pdf", year, schoolCode, upload.SHA256Hex)
	if err := h.putStaged(r, upload, storageKey); err != nil {
		writeIngestionError(w, http.StatusBadGateway, "storage_unavailable", "extraction storage is unavailable")
		return
	}
	document, oldStorageKey, err := h.brochureRepository.CreateBrochureSystem(r.Context(), admissions.BrochureDocumentInput{
		AcademicYear: year, SchoolCode: schoolCode, OriginalFileName: upload.OriginalName,
		StorageKey: storageKey, MIMEType: upload.ContentType, FileSizeBytes: upload.Size,
		SHA256: upload.SHA256Hex, SourceURL: sourceURL,
	})
	if err != nil {
		// Keep an orphaned source recoverable for operators to clean up instead
		// of risking deletion of a key still referenced by an older row.
		h.writeExternalError(w, err)
		return
	}
	if oldStorageKey != "" && oldStorageKey != storageKey {
		_ = h.blobStore.Remove(r.Context(), oldStorageKey)
	}
	jobID, dispatchErr := h.service.QueueExternalBrochureExtraction(r.Context(), year, schoolCode, storageKey, upload.SHA256Hex, sourceURL)
	h.writeQueuedResponse(w, document, jobs.SourceTypeBrochure, year, schoolCode, jobID, dispatchErr)
}

func (h *ExternalHandler) uploadCandidateList(w http.ResponseWriter, r *http.Request, adminID *uuid.UUID) {
	if h.blobStore == nil {
		writeIngestionError(w, http.StatusServiceUnavailable, "storage_unavailable", "extraction storage is unavailable")
		return
	}
	upload, year, schoolCode, sourceURL, programCode, err := h.parseFileUpload(w, r, jobs.SourceTypeCandidateList)
	if err != nil {
		return
	}
	defer upload.Close()
	if !allowedCandidateListType(upload.ContentType) {
		writeIngestionError(w, http.StatusBadRequest, "invalid_file", "candidate list must be a PDF, CSV, TSV, text, or JSON file")
		return
	}
	extension := strings.ToLower(filepath.Ext(upload.OriginalName))
	storageKey := fmt.Sprintf("candidate-lists/%03d-%s-%s%s", year, schoolCode, upload.SHA256Hex, extension)
	if err := h.putStaged(r, upload, storageKey); err != nil {
		writeIngestionError(w, http.StatusBadGateway, "storage_unavailable", "extraction storage is unavailable")
		return
	}
	var jobID uuid.UUID
	if adminID != nil {
		jobID, err = h.service.QueueCandidateListExtraction(r.Context(), *adminID, year, schoolCode, programCode, storageKey, upload.SHA256Hex, sourceURL)
	} else {
		jobID, err = h.service.QueueCandidateListExtractionSystem(r.Context(), year, schoolCode, programCode, storageKey, upload.SHA256Hex, sourceURL)
	}
	if err != nil && !errors.Is(err, ErrDispatchUnavailable) {
		h.writeExternalError(w, err)
		return
	}
	data := map[string]any{
		"job_id": jobID, "document_type": jobs.SourceTypeCandidateList,
		"academic_year": year, "school_code": schoolCode,
		"program_code": programCode, "source_sha256": upload.SHA256Hex,
		"extraction_status": "queued",
	}
	status := http.StatusCreated
	if errors.Is(err, ErrDispatchUnavailable) {
		status = http.StatusAccepted
		data["extraction_status"] = "retrying"
	}
	writeIngestionJSON(w, status, map[string]any{"data": data})
}

func (h *ExternalHandler) parseFileUpload(w http.ResponseWriter, r *http.Request, sourceType string) (storage.StagedUpload, int, string, string, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxPortfolioFileBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_multipart", "extraction upload is invalid")
		return storage.StagedUpload{}, 0, "", "", "", err
	}
	defer r.MultipartForm.RemoveAll()
	year, err := strconv.Atoi(strings.TrimSpace(r.FormValue("academic_year")))
	schoolCode := strings.TrimSpace(r.FormValue("school_code"))
	if err != nil || year < 100 || year > 999 || !threeDigitCode(schoolCode) || schoolCode == "000" {
		writeIngestionError(w, http.StatusBadRequest, "invalid_source", "academic year or school code is invalid")
		return storage.StagedUpload{}, 0, "", "", "", ErrInvalid
	}
	sourceURL := strings.TrimSpace(r.FormValue("source_url"))
	if admissions.ValidateOfficialURL(sourceURL) != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_source_url", "source URL must be an official .edu.tw or .gov.tw URL")
		return storage.StagedUpload{}, 0, "", "", "", ErrInvalid
	}
	programCode := strings.TrimSpace(r.FormValue("program_code"))
	if programCode != "" && !threeDigitCode(programCode) {
		writeIngestionError(w, http.StatusBadRequest, "invalid_program_code", "program code is invalid")
		return storage.StagedUpload{}, 0, "", "", "", ErrInvalid
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "file_required", "extraction source file is required")
		return storage.StagedUpload{}, 0, "", "", "", err
	}
	defer file.Close()
	upload, err := storage.StageUpload(header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_file", "source file is invalid")
		return storage.StagedUpload{}, 0, "", "", "", err
	}
	if sourceType == jobs.SourceTypeBrochure && upload.ContentType != "application/pdf" {
		upload.Close()
		writeIngestionError(w, http.StatusBadRequest, "invalid_file", "brochure must be a valid PDF")
		return storage.StagedUpload{}, 0, "", "", "", ErrInvalid
	}
	if err := storage.ScanStagedUpload(r.Context(), h.scanner, upload); err != nil {
		upload.Close()
		if errors.Is(err, storage.ErrMalwareDetected) {
			writeIngestionError(w, http.StatusUnprocessableEntity, "malware_detected", "source file was rejected")
		} else {
			writeIngestionError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
		}
		return storage.StagedUpload{}, 0, "", "", "", err
	}
	return upload, year, schoolCode, sourceURL, programCode, nil
}

func (h *ExternalHandler) putStaged(r *http.Request, upload storage.StagedUpload, storageKey string) error {
	file, err := os.Open(upload.Path)
	if err != nil {
		return err
	}
	defer file.Close()
	return h.blobStore.Put(r.Context(), storageKey, file, upload.Size, upload.ContentType)
}

func (h *ExternalHandler) writeQueuedResponse(w http.ResponseWriter, document admissions.BrochureDocument, sourceType string, year int, schoolCode string, jobID uuid.UUID, dispatchErr error) {
	status := http.StatusCreated
	extractionStatus := "queued"
	if errors.Is(dispatchErr, ErrDispatchUnavailable) {
		status = http.StatusAccepted
		extractionStatus = "retrying"
	} else if dispatchErr != nil {
		h.writeExternalError(w, dispatchErr)
		return
	}
	data := map[string]any{
		"document": document, "job_id": jobID, "document_type": sourceType,
		"academic_year": year, "school_code": schoolCode,
		"source_sha256": document.SHA256, "extraction_status": extractionStatus,
	}
	writeIngestionJSON(w, status, map[string]any{"data": data})
}

func allowedCandidateListType(contentType string) bool {
	switch contentType {
	case "application/pdf", "text/csv", "text/tab-separated-values", "text/plain", "application/json":
		return true
	default:
		return false
	}
}

func threeDigitCode(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func (h *ExternalHandler) claim(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w, r) {
		return
	}
	if h.blobStore == nil {
		writeIngestionError(w, http.StatusServiceUnavailable, "storage_unavailable", "extraction storage is unavailable")
		return
	}
	var input struct {
		DocumentType string `json:"document_type"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeIngestionJSON(r, &input); err != nil {
			writeIngestionError(w, http.StatusBadRequest, "invalid_claim", "extraction claim is invalid")
			return
		}
	}
	sourceType := strings.TrimSpace(input.DocumentType)
	if sourceType == "" {
		sourceType = jobs.SourceTypeBrochure
	}
	job, err := h.repository.ClaimNextJob(r.Context(), sourceType, 15*time.Minute)
	if errors.Is(err, ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		h.writeExternalError(w, err)
		return
	}
	downloadURL, err := h.blobStore.PresignGet(r.Context(), job.StorageKey, 5*time.Minute)
	if err != nil {
		writeIngestionError(w, http.StatusBadGateway, "storage_unavailable", "extraction source is unavailable")
		_ = h.repository.MarkJobFailure(r.Context(), job.JobID, JobFailureInput{Code: "source_unavailable", Message: "extraction source is unavailable", Retryable: true})
		return
	}
	writeIngestionJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"job": job, "download_url": downloadURL.String(), "expires_in": 300,
	}})
}

func (h *ExternalHandler) failure(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w, r) {
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_job_id", "job id is invalid")
		return
	}
	var input JobFailureInput
	if err := decodeIngestionJSON(r, &input); err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_failure", "extraction failure is invalid")
		return
	}
	if err := h.repository.MarkJobFailure(r.Context(), jobID, input); err != nil {
		h.writeExternalError(w, err)
		return
	}
	writeIngestionJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"job_id": jobID, "status": "recorded"}})
}

func (h *ExternalHandler) jobStatus(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w, r) {
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_job_id", "job id is invalid")
		return
	}
	item, err := h.repository.GetJobStatus(r.Context(), jobID)
	if errors.Is(err, ErrNotFound) {
		writeIngestionError(w, http.StatusNotFound, "not_found", "extraction job was not found")
		return
	}
	if err != nil {
		h.writeExternalError(w, err)
		return
	}
	writeIngestionJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *ExternalHandler) result(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w, r) {
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_job_id", "job id is invalid")
		return
	}
	var raw json.RawMessage
	if err := decodeIngestionJSON(r, &raw); err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_result", "extraction result is invalid")
		return
	}
	var envelope struct {
		ResultType string    `json:"result_type"`
		JobID      uuid.UUID `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.JobID != jobID {
		writeIngestionError(w, http.StatusBadRequest, "invalid_result", "extraction result does not match the job")
		return
	}
	if envelope.ResultType == jobs.SourceTypeCandidateList {
		var result jobs.CandidateListExtractionResult
		if err := json.Unmarshal(raw, &result); err != nil || result.Validate() != nil {
			writeIngestionError(w, http.StatusBadRequest, "invalid_result", "candidate list extraction result is invalid")
			return
		}
		if err := h.resultApplier.ApplyCandidateListExtractionResult(r.Context(), result); err != nil {
			h.writeExternalError(w, err)
			return
		}
	} else {
		var result jobs.BrochureExtractionResult
		if err := json.Unmarshal(raw, &result); err != nil || result.Validate() != nil {
			writeIngestionError(w, http.StatusBadRequest, "invalid_result", "brochure extraction result is invalid")
			return
		}
		if err := h.repository.ApplyExtractionResult(r.Context(), result); err != nil {
			h.writeExternalError(w, err)
			return
		}
	}
	writeIngestionJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"job_id": jobID, "status": "succeeded"}})
}

func (h *ExternalHandler) requireService(w http.ResponseWriter, r *http.Request) bool {
	if h.serviceToken == "" {
		writeIngestionError(w, http.StatusServiceUnavailable, "service_unavailable", "external extraction API is not configured")
		return false
	}
	provided := bearerToken(r.Header.Get("Authorization"))
	expectedHash := sha256.Sum256([]byte(h.serviceToken))
	providedHash := sha256.Sum256([]byte(provided))
	if provided == "" || subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) != 1 {
		writeIngestionError(w, http.StatusUnauthorized, "invalid_service_token", "service authentication failed")
		return false
	}
	return true
}

func bearerToken(value string) string {
	parts := strings.Fields(value)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}
	return ""
}

func (h *ExternalHandler) requireAdminMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeIngestionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		} else {
			writeIngestionError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		}
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeIngestionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeIngestionError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeIngestionError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeIngestionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *ExternalHandler) writeExternalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid), errors.Is(err, admissions.ErrInvalidProgram), errors.Is(err, results.ErrInvalidInput):
		writeIngestionError(w, http.StatusBadRequest, "invalid_extraction", "extraction input is invalid")
	case errors.Is(err, ErrNotFound), errors.Is(err, admissions.ErrNotFound), errors.Is(err, results.ErrNotFound):
		writeIngestionError(w, http.StatusNotFound, "not_found", "extraction resource was not found")
	case errors.Is(err, ErrConflict), errors.Is(err, results.ErrConflict), errors.Is(err, results.ErrInvalidStatus):
		writeIngestionError(w, http.StatusConflict, "conflict", "extraction resource conflicts with existing data")
	case errors.Is(err, results.ErrAdminRequired):
		writeIngestionError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrDispatchUnavailable):
		writeIngestionError(w, http.StatusAccepted, "dispatch_unavailable", "extraction was stored and will be retried")
	default:
		writeIngestionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
