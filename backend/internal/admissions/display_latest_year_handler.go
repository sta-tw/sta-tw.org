package admissions

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type displayLatestAcademicYearSettingsRepository interface {
	GetDisplayLatestAcademicYear(context.Context) (int, bool, error)
	SetDisplayLatestAcademicYear(context.Context, uuid.UUID, int) error
}

func (h *AdminHandler) getDisplayLatestAcademicYear(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	repository, ok := h.repository.(displayLatestAcademicYearSettingsRepository)
	if !ok {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "display year settings are unavailable")
		return
	}
	year, isSet, err := repository.GetDisplayLatestAcademicYear(r.Context())
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"academic_year": year, "is_set": isSet},
	})
}

func (h *AdminHandler) setDisplayLatestAcademicYear(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	var body struct {
		AcademicYear int `json:"academic_year"`
	}
	if err := decodeBrochureJSON(r, &body); err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	if body.AcademicYear < 100 || body.AcademicYear > 999 {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_academic_year", "academic_year must be a three-digit value")
		return
	}
	repository, ok := h.repository.(displayLatestAcademicYearSettingsRepository)
	if !ok {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "display year settings are unavailable")
		return
	}
	if err := repository.SetDisplayLatestAcademicYear(r.Context(), session.Session.Account.ID, body.AcademicYear); err != nil {
		h.writeAdminError(w, err)
		return
	}
	h.invalidate(r.Context(), "display-latest-year")

	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"academic_year": body.AcademicYear, "is_set": true},
	})
}
