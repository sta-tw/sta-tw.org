package content

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/pagination"
)

type Handler struct {
	authService *auth.Service
	repository  Repository
}

func NewHandler(authService *auth.Service, repository Repository) (*Handler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("content handler dependencies are missing")
	}
	return &Handler{authService: authService, repository: repository}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/forum/spaces", h.listSpaces)
	mux.HandleFunc("POST /api/v1/forum/spaces/{spaceID}/join", h.joinSpace)
	mux.HandleFunc("POST /api/v1/forum/spaces/{spaceID}/leave", h.leaveSpace)
	mux.HandleFunc("GET /api/v1/forum/spaces/{spaceID}/threads", h.listThreads)
	mux.HandleFunc("POST /api/v1/forum/spaces/{spaceID}/threads", h.createThread)
	mux.HandleFunc("GET /api/v1/forum/threads/{threadID}/posts", h.listPosts)
	mux.HandleFunc("POST /api/v1/forum/threads/{threadID}/posts", h.createPost)
	mux.HandleFunc("GET /api/v1/experiences", h.listExperiences)
	mux.HandleFunc("GET /api/v1/experiences/{experienceID}", h.getExperience)
	mux.HandleFunc("POST /api/v1/experiences", h.createExperience)
	mux.HandleFunc("POST /api/v1/experiences/{experienceID}/revisions", h.createRevision)
	mux.HandleFunc("POST /api/v1/experience-revisions/{revisionID}/submit", h.submitRevision)
	mux.HandleFunc("POST /api/v1/experiences/{experienceID}/unpublish", h.unpublishExperience)
	mux.HandleFunc("POST /api/v1/admin/experience-revisions/{revisionID}/review", h.reviewExperience)
	mux.HandleFunc("PUT /api/v1/forum/posts/{postID}/reactions/{emoji}", h.addPostReaction)
	mux.HandleFunc("DELETE /api/v1/forum/posts/{postID}/reactions/{emoji}", h.removePostReaction)
	mux.HandleFunc("PUT /api/v1/experiences/{experienceID}/reactions/{emoji}", h.addExperienceReaction)
	mux.HandleFunc("DELETE /api/v1/experiences/{experienceID}/reactions/{emoji}", h.removeExperienceReaction)
}

func (h *Handler) listSpaces(w http.ResponseWriter, r *http.Request) {
	accountID := h.optionalAccountID(r)
	spaces, err := h.repository.ListSpaces(r.Context(), accountID)
	if err != nil {
		writeContentError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeContentJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

func (h *Handler) joinSpace(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	spaceID, err := uuid.Parse(r.PathValue("spaceID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_space_id", "space id is invalid")
		return
	}
	if err := h.repository.JoinSpace(r.Context(), session.Session.Account.ID, spaceID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) leaveSpace(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	spaceID, err := uuid.Parse(r.PathValue("spaceID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_space_id", "space id is invalid")
		return
	}
	if err := h.repository.LeaveSpace(r.Context(), session.Session.Account.ID, spaceID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listThreads(w http.ResponseWriter, r *http.Request) {
	spaceID, err := uuid.Parse(r.PathValue("spaceID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_space_id", "space id is invalid")
		return
	}
	limit, cursor, ok := contentPageQuery(w, r)
	if !ok {
		return
	}
	threads, nextCursor, err := h.repository.ListThreads(r.Context(), h.optionalAccountID(r), spaceID, limit, cursor)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeContentJSON(w, http.StatusOK, map[string]any{"data": threads, "next_cursor": nextCursor})
}

func (h *Handler) listPosts(w http.ResponseWriter, r *http.Request) {
	threadID, err := uuid.Parse(r.PathValue("threadID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_thread_id", "thread id is invalid")
		return
	}
	limit, cursor, ok := contentPageQuery(w, r)
	if !ok {
		return
	}
	posts, nextCursor, err := h.repository.ListPosts(r.Context(), h.optionalAccountID(r), threadID, limit, cursor)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeContentJSON(w, http.StatusOK, map[string]any{"data": posts, "next_cursor": nextCursor})
}

func (h *Handler) createThread(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	spaceID, err := uuid.Parse(r.PathValue("spaceID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_space_id", "space id is invalid")
		return
	}
	var input struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := decodeContentJSON(r, &input); err != nil || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Body) == "" || len(input.Title) > 300 || len(input.Body) > 100000 {
		writeContentError(w, http.StatusBadRequest, "invalid_thread", "thread data is invalid")
		return
	}
	thread, post, err := h.repository.CreateThread(r.Context(), session.Session.Account.ID, spaceID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Body))
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeContentJSON(w, http.StatusCreated, map[string]any{"thread": thread, "first_post": post})
}

func (h *Handler) createPost(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	threadID, err := uuid.Parse(r.PathValue("threadID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_thread_id", "thread id is invalid")
		return
	}
	var input struct {
		Body               string     `json:"body"`
		QuotedExperienceID *uuid.UUID `json:"quoted_experience_id"`
	}
	if err := decodeContentJSON(r, &input); err != nil || strings.TrimSpace(input.Body) == "" || len(input.Body) > 100000 {
		writeContentError(w, http.StatusBadRequest, "invalid_post", "post data is invalid")
		return
	}
	post, err := h.repository.CreatePost(r.Context(), session.Session.Account.ID, threadID, strings.TrimSpace(input.Body), input.QuotedExperienceID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeContentJSON(w, http.StatusCreated, map[string]any{"data": post})
}

func (h *Handler) listExperiences(w http.ResponseWriter, r *http.Request) {
	limit, cursor, ok := contentPageQuery(w, r)
	if !ok {
		return
	}
	experiences, nextCursor, err := h.repository.ListPublishedExperiences(r.Context(), limit, cursor)
	if err != nil {
		writeContentError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeContentJSON(w, http.StatusOK, map[string]any{"data": experiences, "next_cursor": nextCursor})
}

func (h *Handler) getExperience(w http.ResponseWriter, r *http.Request) {
	experienceID, err := uuid.Parse(r.PathValue("experienceID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_experience_id", "experience id is invalid")
		return
	}
	var accountID *uuid.UUID
	if session, authErr := h.authService.Authenticate(r.Context(), r); authErr == nil {
		id := session.Session.Account.ID
		accountID = &id
	}
	experience, err := h.repository.GetExperience(r.Context(), accountID, experienceID)
	if errors.Is(err, ErrNotFound) {
		writeContentError(w, http.StatusNotFound, "not_found", "experience not found")
		return
	}
	if err != nil {
		writeContentError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeContentJSON(w, http.StatusOK, map[string]any{"data": experience})
}

func (h *Handler) createExperience(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	var input CreateExperienceInput
	if err := decodeContentJSON(r, &input); err != nil || ValidateText(input.Title, input.Body) != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_experience", "experience data is invalid")
		return
	}
	experience, err := h.repository.CreateExperience(r.Context(), session.Session.Account.ID, input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeContentJSON(w, http.StatusCreated, map[string]any{"data": experience})
}

func (h *Handler) createRevision(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	experienceID, err := uuid.Parse(r.PathValue("experienceID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_experience_id", "experience id is invalid")
		return
	}
	var input CreateRevisionInput
	if err := decodeContentJSON(r, &input); err != nil || ValidateText(input.Title, input.Body) != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_revision", "revision data is invalid")
		return
	}
	experience, err := h.repository.CreateRevision(r.Context(), session.Session.Account.ID, experienceID, input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeContentJSON(w, http.StatusCreated, map[string]any{"data": experience})
}

func (h *Handler) submitRevision(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	revisionID, err := uuid.Parse(r.PathValue("revisionID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_revision_id", "revision id is invalid")
		return
	}
	experience, err := h.repository.SubmitRevision(r.Context(), session.Session.Account.ID, revisionID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeContentJSON(w, http.StatusOK, map[string]any{"data": experience})
}

func (h *Handler) unpublishExperience(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	experienceID, err := uuid.Parse(r.PathValue("experienceID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_experience_id", "experience id is invalid")
		return
	}
	if err := h.repository.UnpublishExperience(r.Context(), session.Session.Account.ID, experienceID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reviewExperience(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	revisionID, err := uuid.Parse(r.PathValue("revisionID"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_revision_id", "revision id is invalid")
		return
	}
	var input ReviewInput
	if err := decodeContentJSON(r, &input); err != nil || (!input.Approved && strings.TrimSpace(input.Reason) == "") || len(input.Reason) > 2000 {
		writeContentError(w, http.StatusBadRequest, "invalid_review", "review data is invalid")
		return
	}
	experience, err := h.repository.ReviewExperience(r.Context(), session.Session.Account.ID, revisionID, input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeContentJSON(w, http.StatusOK, map[string]any{"data": experience})
}

func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeContentError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeContentError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) optionalAccountID(r *http.Request) *uuid.UUID {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		return nil
	}
	id := session.Session.Account.ID
	return &id
}

func (h *Handler) requireMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireAuth(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeContentError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeContentError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeContentError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeContentError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) addPostReaction(w http.ResponseWriter, r *http.Request) {
	h.reactionOp(w, r, ReactionTargetPost, "postID", true)
}
func (h *Handler) removePostReaction(w http.ResponseWriter, r *http.Request) {
	h.reactionOp(w, r, ReactionTargetPost, "postID", false)
}
func (h *Handler) addExperienceReaction(w http.ResponseWriter, r *http.Request) {
	h.reactionOp(w, r, ReactionTargetExperience, "experienceID", true)
}
func (h *Handler) removeExperienceReaction(w http.ResponseWriter, r *http.Request) {
	h.reactionOp(w, r, ReactionTargetExperience, "experienceID", false)
}

func (h *Handler) reactionOp(w http.ResponseWriter, r *http.Request, targetType, idParam string, add bool) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	targetID, err := uuid.Parse(r.PathValue(idParam))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_id", "target id is invalid")
		return
	}
	emoji, err := NormalizeReaction(r.PathValue("emoji"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_reaction", "reaction is invalid")
		return
	}
	if add {
		err = h.repository.SetReaction(r.Context(), targetType, targetID, session.Session.Account.ID, emoji)
	} else {
		err = h.repository.RemoveReaction(r.Context(), targetType, targetID, session.Session.Account.ID, emoji)
	}
	if errors.Is(err, ErrInvalidReaction) {
		writeContentError(w, http.StatusBadRequest, "invalid_reaction", "reaction is invalid")
		return
	}
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeContentError(w, http.StatusNotFound, "not_found", "content resource not found")
	case errors.Is(err, ErrForbidden):
		writeContentError(w, http.StatusForbidden, "forbidden", "content access is not allowed")
	case errors.Is(err, ErrConflict):
		writeContentError(w, http.StatusConflict, "conflict", "content resource conflict")
	case errors.Is(err, ErrInvalidStatus):
		writeContentError(w, http.StatusConflict, "invalid_status", "content status transition is invalid")
	case errors.Is(err, ErrAdminRequired):
		writeContentError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	default:
		writeContentError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func decodeContentJSON(r *http.Request, destination any) error {
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

type contentErrorBody struct {
	Error contentError `json:"error"`
}
type contentError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeContentJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeContentError(w http.ResponseWriter, status int, code, message string) {
	writeContentJSON(w, status, contentErrorBody{Error: contentError{Code: code, Message: message}})
}

// contentPageQuery reads the shared ?limit=&cursor= keyset pagination inputs.
// On invalid input it writes a 400 and returns ok=false.
func contentPageQuery(w http.ResponseWriter, r *http.Request) (int, pagination.Cursor, bool) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeContentError(w, http.StatusBadRequest, "invalid_query", "list query is invalid")
			return 0, pagination.Cursor{}, false
		}
		limit = parsed
	}
	cursor, err := pagination.Decode(r.URL.Query().Get("cursor"))
	if err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_query", "list query is invalid")
		return 0, pagination.Cursor{}, false
	}
	return limit, cursor, true
}
