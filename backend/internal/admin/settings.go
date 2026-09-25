package admin

import (
	"net/http"
)

// getRequireAdminMFA / setRequireAdminMFA expose the app_settings toggle
// auth.Service.effectiveRequireAdminMFA reads — lets an operator turn the
// admin-MFA gate on/off without a redeploy. Off is the safe default while
// the enrollment UI (binding an authenticator) doesn't exist yet: forcing
// the gate on with no way to enroll just locks every admin out.
func (h *Handler) getRequireAdminMFA(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	var value string
	err := h.pool.QueryRow(r.Context(), `SELECT value FROM app_settings WHERE key = 'require_admin_mfa'`).Scan(&value)
	if err != nil {
		// No row yet (or a transient error) — report the same "not
		// currently enforced" state RequireAdminMFA falls back to.
		writeJSON(w, http.StatusOK, map[string]any{"value": false, "is_set": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": value == "true", "is_set": true})
}

func (h *Handler) setRequireAdminMFA(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	var body struct {
		Value bool `json:"value"`
	}
	if err := decodeAdminJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	stored := "false"
	if body.Value {
		stored = "true"
	}
	if _, err := h.pool.Exec(r.Context(), `
		INSERT INTO app_settings (key, value, updated_by)
		VALUES ('require_admin_mfa', $1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP, updated_by = EXCLUDED.updated_by
	`, stored, session.Session.Account.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": body.Value})
}
