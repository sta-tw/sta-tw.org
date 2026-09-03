package admissions

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	repository Repository
}

func NewHandler(repository Repository) (*Handler, error) {
	if repository == nil {
		return nil, errors.New("admission repository is nil")
	}
	return &Handler{repository: repository}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admissions/schools", h.listSchools)
	mux.HandleFunc("GET /api/v1/admissions/programs", h.listPrograms)
	mux.HandleFunc("GET /api/v1/admissions/programs/{identifier}", h.getProgram)
}

func (h *Handler) listSchools(w http.ResponseWriter, r *http.Request) {
	year, err := parseYear(r.URL.Query().Get("academic_year"))
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_academic_year", "academic_year is invalid")
		return
	}
	schools, err := h.repository.ListSchools(r.Context(), year)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": schools})
}

func (h *Handler) listPrograms(w http.ResponseWriter, r *http.Request) {
	query, err := parseProgramQuery(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_query", "admission query is invalid")
		return
	}
	programs, err := h.repository.ListPrograms(r.Context(), query)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{
		"data": programs,
		"meta": map[string]any{"limit": query.Limit, "offset": query.Offset},
	})
}

func (h *Handler) getProgram(w http.ResponseWriter, r *http.Request) {
	identifier, err := ParseProgramIdentifier(r.PathValue("identifier"))
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program_identifier", "program identifier is invalid")
		return
	}
	program, err := h.repository.GetProgram(r.Context(), identifier)
	if errors.Is(err, ErrNotFound) {
		writeAdmissionError(w, http.StatusNotFound, "not_found", "admission program not found")
		return
	}
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": program})
}

func parseProgramQuery(r *http.Request) (ProgramQuery, error) {
	year, err := parseYear(r.URL.Query().Get("academic_year"))
	if err != nil {
		return ProgramQuery{}, err
	}
	schoolCode := strings.TrimSpace(r.URL.Query().Get("school_code"))
	if schoolCode != "" && !codePattern.MatchString(schoolCode) {
		return ProgramQuery{}, ErrInvalidProgram
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 100 {
		return ProgramQuery{}, ErrInvalidProgram
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return ProgramQuery{}, ErrInvalidProgram
		}
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 10000 {
			return ProgramQuery{}, ErrInvalidProgram
		}
	}
	return ProgramQuery{AcademicYear: year, SchoolCode: schoolCode, Search: search, Limit: limit, Offset: offset}, nil
}

func parseYear(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	year, err := strconv.Atoi(raw)
	if err != nil || year < 100 || year > 999 {
		return 0, ErrInvalidProgram
	}
	return year, nil
}

type admissionErrorBody struct {
	Error admissionError `json:"error"`
}

type admissionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeAdmissionJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAdmissionError(w http.ResponseWriter, status int, code, message string) {
	writeAdmissionJSON(w, status, admissionErrorBody{Error: admissionError{Code: code, Message: message}})
}
