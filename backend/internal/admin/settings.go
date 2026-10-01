package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// DefaultMailReplyTemplate is used whenever no admin has customized it yet
// (see internal/emailinquiries.Service, which reads the same app_settings
// row this handler writes). [內容] and [簽名] are the two placeholders a
// /re reply's message and required signature get substituted into.
const DefaultMailReplyTemplate = "您好，感謝您的來信，回覆如下：\n\n[內容]\n\n如有其他問題，歡迎再次與我們聯繫。\n\n[簽名]"

const (
	sitePolicyTermsKey   = "site_policy_terms"
	sitePolicyPrivacyKey = "site_policy_privacy"
	maxSitePolicyBytes   = 500_000
)

type sitePolicyDocument struct {
	Value     string     `json:"value"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type sitePolicyResponse struct {
	Terms   sitePolicyDocument `json:"terms"`
	Privacy sitePolicyDocument `json:"privacy"`
}

func sitePolicySettingKey(name string) (string, bool) {
	switch strings.TrimSpace(name) {
	case "terms":
		return sitePolicyTermsKey, true
	case "privacy":
		return sitePolicyPrivacyKey, true
	default:
		return "", false
	}
}

func (h *Handler) getSitePolicies(w http.ResponseWriter, r *http.Request) {
	response, err := h.readSitePolicies(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": response})
}

func (h *Handler) getAdminSitePolicies(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	response, err := h.readSitePolicies(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": response})
}

func (h *Handler) readSitePolicies(r *http.Request) (sitePolicyResponse, error) {
	response := sitePolicyResponse{}
	rows, err := h.pool.Query(r.Context(), `
		SELECT key, value, updated_at
		FROM app_settings
		WHERE key IN ($1, $2)
	`, sitePolicyTermsKey, sitePolicyPrivacyKey)
	if err != nil {
		return response, err
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var document sitePolicyDocument
		if err := rows.Scan(&key, &document.Value, &document.UpdatedAt); err != nil {
			return response, err
		}
		switch key {
		case sitePolicyTermsKey:
			response.Terms = document
		case sitePolicyPrivacyKey:
			response.Privacy = document
		}
	}
	return response, rows.Err()
}

func (h *Handler) setSitePolicy(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}

	key, valid := sitePolicySettingKey(r.PathValue("policy"))
	if !valid {
		writeError(w, http.StatusNotFound, "not_found", "site policy not found")
		return
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := decodeAdminJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	value := strings.TrimSpace(body.Value)
	if value == "" {
		writeError(w, http.StatusBadRequest, "invalid_policy", "policy content cannot be empty")
		return
	}
	if len([]byte(value)) > maxSitePolicyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "policy_too_large", "policy content is too large")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		slog.Default().ErrorContext(r.Context(), "site policy save: begin tx", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	defer tx.Rollback(r.Context())

	var before string
	err = tx.QueryRow(r.Context(), `SELECT value FROM app_settings WHERE key = $1 FOR UPDATE`, key).Scan(&before)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Default().ErrorContext(r.Context(), "site policy save: select current value", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO app_settings (key, value, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			updated_at = CURRENT_TIMESTAMP,
			updated_by = EXCLUDED.updated_by
	`, key, value, session.Session.Account.ID); err != nil {
		slog.Default().ErrorContext(r.Context(), "site policy save: upsert app_settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO audit_log (
			actor_account_id, action, entity_type, entity_key, before_data, after_data, reason
		)
		VALUES (
			$1,
			'site_policy.updated',
			'site_policy',
			$2,
			CASE WHEN $3 = '' THEN NULL ELSE jsonb_build_object('value', $3::text) END,
			jsonb_build_object('value', $4::text),
			'Updated from the admin policy editor'
		)
	`, session.Session.Account.ID, strings.TrimPrefix(key, "site_policy_"), before, value); err != nil {
		slog.Default().ErrorContext(r.Context(), "site policy save: insert audit_log", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		slog.Default().ErrorContext(r.Context(), "site policy save: commit tx", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"value": value})
}

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

// getMailReplyTemplate / setMailReplyTemplate expose the app_settings row
// internal/emailinquiries.Service.ReplyByDiscord reads to build every /re
// reply's email body — see DefaultMailReplyTemplate for the fallback shape
// and internal/discordmail for how [內容]/[簽名] get filled in.
func (h *Handler) getMailReplyTemplate(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	var value string
	err := h.pool.QueryRow(r.Context(), `SELECT value FROM app_settings WHERE key = 'mail_reply_template'`).Scan(&value)
	if err != nil {
		value = DefaultMailReplyTemplate
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": value})
}

func (h *Handler) setMailReplyTemplate(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := decodeAdminJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	value := strings.TrimSpace(body.Value)
	if !strings.Contains(value, "[內容]") || !strings.Contains(value, "[簽名]") {
		writeError(w, http.StatusBadRequest, "invalid_template", "template must include both [內容] and [簽名] placeholders")
		return
	}
	if _, err := h.pool.Exec(r.Context(), `
		INSERT INTO app_settings (key, value, updated_by)
		VALUES ('mail_reply_template', $1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP, updated_by = EXCLUDED.updated_by
	`, value, session.Session.Account.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": value})
}
