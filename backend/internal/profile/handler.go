package profile

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"sta-backend/internal/auth"
	"sta-backend/internal/security"
	"sta-backend/internal/storage"
)

const (
	maxAvatarBytes   int64 = 2 << 20
	avatarPresignTTL       = 5 * time.Minute
)

var allowedAvatarTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
}

type Handler struct {
	auth          *auth.Service
	repo          Repository
	blobStore     storage.BlobStore
	scanner       storage.Scanner
	uploadLimiter *security.FixedWindowLimiter
}

func NewHandler(authService *auth.Service, repo Repository, blobStore storage.BlobStore, scanner storage.Scanner) (*Handler, error) {
	if authService == nil || repo == nil || blobStore == nil {
		return nil, errors.New("profile handler dependencies are missing")
	}
	return &Handler{
		auth:          authService,
		repo:          repo,
		blobStore:     blobStore,
		scanner:       scanner,
		uploadLimiter: security.NewFixedWindowLimiter(10, time.Minute, 10000),
	}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/profile", h.getSelf)
	mux.HandleFunc("PUT /api/v1/profile", h.updateSelf)
	mux.HandleFunc("POST /api/v1/profile/avatar", h.uploadAvatar)
	mux.HandleFunc("DELETE /api/v1/profile/avatar", h.deleteAvatar)
	mux.HandleFunc("GET /api/v1/profile/avatar", h.getSelfAvatar)
	mux.HandleFunc("GET /api/v1/users/{username}", h.getPublic)
	mux.HandleFunc("GET /api/v1/users/{username}/avatar", h.getPublicAvatar)
}

func (h *Handler) authed(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.auth.Authenticate(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) authedMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.authed(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.auth.AuthorizeMutation(r, session); err != nil {
		writeError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) getSelf(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authed(w, r)
	if !ok {
		return
	}
	p, err := h.repo.Get(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": p})
}

func (h *Handler) updateSelf(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authedMutation(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var in Input
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	if err := in.Normalize(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", "profile fields are invalid")
		return
	}
	p, err := h.repo.Upsert(r.Context(), session.Session.Account.ID, in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": p})
}

func (h *Handler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authedMutation(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	rl := h.uploadLimiter.Take(session.Session.Account.ID.String(), now)
	security.WriteRateLimitHeaders(w, rl, now)
	if !rl.Allowed {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many avatar uploads")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_multipart", "multipart request is invalid")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required", "file is required")
		return
	}
	defer file.Close()

	staged, err := storage.StageUploadWithLimit(header.Filename, header.Header.Get("Content-Type"), file, maxAvatarBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_file", "avatar must be a PNG or JPEG under 2 MB")
		return
	}
	defer staged.Close()
	if _, ok := allowedAvatarTypes[staged.ContentType]; !ok {
		writeError(w, http.StatusBadRequest, "invalid_file", "avatar must be a PNG or JPEG")
		return
	}
	if err := storage.ScanStagedUpload(r.Context(), h.scanner, staged); err != nil {
		if errors.Is(err, storage.ErrMalwareDetected) {
			writeError(w, http.StatusUnprocessableEntity, "malware_detected", "uploaded file was rejected")
		} else {
			writeError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
		}
		return
	}

	storageKey := "avatars/" + session.Session.Account.ID.String() + "/" + uuid.NewString()
	body, err := os.Open(staged.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	putErr := h.blobStore.Put(r.Context(), storageKey, body, staged.Size, staged.ContentType)
	_ = body.Close()
	if putErr != nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		writeError(w, http.StatusBadGateway, "storage_unavailable", "file storage is unavailable")
		return
	}
	oldKey, err := h.repo.SetAvatar(r.Context(), session.Session.Account.ID, storageKey, staged.ContentType)
	if err != nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if oldKey != "" {
		_ = h.blobStore.Remove(r.Context(), oldKey)
	}
	p, err := h.repo.Get(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": p})
}

func (h *Handler) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authedMutation(w, r)
	if !ok {
		return
	}
	oldKey, err := h.repo.ClearAvatar(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if oldKey != "" {
		_ = h.blobStore.Remove(r.Context(), oldKey)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getSelfAvatar(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authed(w, r)
	if !ok {
		return
	}
	key, err := h.repo.AvatarByAccountID(r.Context(), session.Session.Account.ID)
	h.redirectToAvatar(w, r, key, err)
}

func (h *Handler) getPublic(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authed(w, r); !ok {
		return
	}
	username := strings.TrimSpace(r.PathValue("username"))
	if username == "" || len(username) > 64 {
		writeError(w, http.StatusBadRequest, "invalid_username", "username is invalid")
		return
	}
	p, err := h.repo.GetByUsername(r.Context(), username)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": p})
}

func (h *Handler) getPublicAvatar(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authed(w, r); !ok {
		return
	}
	username := strings.TrimSpace(r.PathValue("username"))
	key, err := h.repo.AvatarByUsername(r.Context(), username)
	h.redirectToAvatar(w, r, key, err)
}

func (h *Handler) redirectToAvatar(w http.ResponseWriter, r *http.Request, key string, lookupErr error) {
	if errors.Is(lookupErr, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no avatar")
		return
	}
	if lookupErr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	signed, err := h.blobStore.PresignGet(r.Context(), key, avatarPresignTTL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "storage_unavailable", "file storage is unavailable")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.Redirect(w, r, signed.String(), http.StatusFound)
}

// --- JSON helpers ---

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var body errorBody
	body.Error.Code = code
	body.Error.Message = message
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
