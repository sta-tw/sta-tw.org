package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"sta-backend/internal/auth"
)

// createUserInput covers both account-creation modes an operator needs:
//
//   - "invite": create an active account for a real person and email them a
//     password-set link (the same "set your password to finish" link
//     RequestPasswordReset always sends) — for onboarding someone outside
//     the normal self-registration flow.
//   - "service": create a 'service'-role (bot) account and issue it one API
//     token, for machine-to-machine access. No email involved; the token
//     is returned once, in this response, and never recoverable afterward.
//   - "password": create an active, human-usable account with an
//     admin-chosen password instead of an emailed link — for a throwaway
//     reviewer account with no real mailbox behind it. The caller must hand
//     the password off out of band; it is never stored anywhere in plaintext.
type createUserInput struct {
	Mode string `json:"mode"`

	// invite
	Username string `json:"username"`
	Email    string `json:"email"`

	// service
	Label      string `json:"label"`
	GrantAdmin bool   `json:"grant_admin"`

	// password
	Password                 string `json:"password"`
	GrantAdmissionsModerator bool   `json:"grant_admissions_moderator"`
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	var body createUserInput
	if err := decodeAdminJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	switch strings.TrimSpace(body.Mode) {
	case "invite":
		h.createInvitedUser(w, r, body)
	case "service":
		h.createServiceUser(w, r, body)
	case "password":
		h.createPasswordUser(w, r, body, session.Session.Account.ID)
	default:
		writeError(w, http.StatusBadRequest, "invalid_mode", "mode must be \"invite\", \"service\", or \"password\"")
	}
}

func (h *Handler) createInvitedUser(w http.ResponseWriter, r *http.Request, body createUserInput) {
	account, err := h.auth.CreateApprovedAccount(r.Context(), body.Username, body.Email)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrConflict):
			writeError(w, http.StatusConflict, "account_conflict", "username or email is already in use")
		case errors.Is(err, auth.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "invalid_input", "username or email is invalid")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	// The account already exists either way; a delivery failure here just
	// means the invitee needs a resend, not that creation should roll back.
	_ = h.auth.RequestPasswordReset(r.Context(), body.Email, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"account": account, "invited": true}})
}

func (h *Handler) createServiceUser(w http.ResponseWriter, r *http.Request, body createUserInput) {
	label := strings.TrimSpace(body.Label)
	if label == "" {
		label = "建立於後台"
	}
	account, token, err := h.auth.CreateBotAccount(r.Context(), body.Username, label)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrConflict):
			writeError(w, http.StatusConflict, "account_conflict", "username is already in use")
		case errors.Is(err, auth.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "invalid_input", "username is invalid")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	if body.GrantAdmin {
		if _, err := h.pool.Exec(r.Context(), `
			INSERT INTO account_roles (account_id, role)
			VALUES ($1, 'admin')
			ON CONFLICT (account_id, role) DO NOTHING
		`, account.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"account": account, "token": token, "granted_admin": body.GrantAdmin,
	}})
}

func (h *Handler) createPasswordUser(w http.ResponseWriter, r *http.Request, body createUserInput, actorID uuid.UUID) {
	account, err := h.auth.CreateAccountWithPassword(r.Context(), body.Username, body.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrConflict):
			writeError(w, http.StatusConflict, "account_conflict", "username is already in use")
		case errors.Is(err, auth.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "invalid_input", "username or password is invalid")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	if body.GrantAdmissionsModerator {
		if err := setAdmissionsModeratorRole(r.Context(), h.pool, account.ID, actorID, true, "建立於後台"); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"account": account, "admissions_moderator": body.GrantAdmissionsModerator,
	}})
}
