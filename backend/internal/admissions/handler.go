package admissions

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Handler struct {
	repository    Repository
	publicBaseURL string
}

func NewHandler(repository Repository, publicBaseURLs ...string) (*Handler, error) {
	if repository == nil {
		return nil, errors.New("admission repository is nil")
	}
	publicBaseURL := "https://sta-tw.org"
	if len(publicBaseURLs) > 0 && strings.TrimSpace(publicBaseURLs[0]) != "" {
		publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURLs[0]), "/")
	}
	return &Handler{repository: repository, publicBaseURL: publicBaseURL}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /sitemap.xml", h.sitemap)
	mux.HandleFunc("GET /llms.txt", h.llms)
	mux.HandleFunc("GET /api/v1/admissions/schools", h.listSchools)
	mux.HandleFunc("GET /api/v1/admissions/programs/search", h.searchPrograms)
	mux.HandleFunc("GET /api/v1/admissions/programs", h.listPrograms)
	mux.HandleFunc("GET /api/v1/admissions/programs/{identifier}", h.getProgram)
}

type sitemapURLSet struct {
	XMLName xml.Name         `xml:"urlset"`
	XMLNS   string           `xml:"xmlns,attr"`
	URLs    []sitemapURLItem `xml:"url"`
}

type sitemapURLItem struct {
	Location string `xml:"loc"`
}

func (h *Handler) sitemap(w http.ResponseWriter, r *http.Request) {
	programs, err := h.allPublishedPrograms(r)
	if err != nil {
		http.Error(w, "Unable to generate sitemap.", http.StatusServiceUnavailable)
		return
	}

	locations := []string{
		h.siteURL("/"),
		h.siteURL("/bochures/"),
		h.siteURL("/articlemain/"),
		h.siteURL("/article/what-is-special-admission/"),
		h.siteURL("/article/who-is-special-admission-for/"),
		h.siteURL("/faq/"),
		h.siteURL("/report-data/"),
	}
	for _, program := range programs {
		locations = append(locations, h.programURL(program.ProgramIdentifier))
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	_ = encoder.Encode(sitemapURLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  makeSitemapItems(locations),
	})
}

func (h *Handler) llms(w http.ResponseWriter, r *http.Request) {
	programs, err := h.allPublishedPrograms(r)
	if err != nil {
		http.Error(w, "Unable to generate llms.txt.", http.StatusServiceUnavailable)
		return
	}

	var content strings.Builder
	content.WriteString("# S.T.A 特殊選才資源網\n\n")
	content.WriteString("> 整理臺灣大學特殊選才招生資料、校系簡章資訊與申請準備文章，協助學生查找招生條件、名額、考試方式及重要時程。\n\n")
	content.WriteString("## 重要頁面\n\n")
	content.WriteString("- [特殊選才簡章與校系搜尋](" + h.siteURL("/bochures/") + "): 依學年度、學校及招生校系查找公開招生資料。\n")
	content.WriteString("- [文章總覽](" + h.siteURL("/articlemain/") + "): 特殊選才準備方向與申請經驗。\n")
	content.WriteString("- [特殊選才是什麼](" + h.siteURL("/article/what-is-special-admission/") + "): 特殊選才的招生目的與制度簡介。\n")
	content.WriteString("- [適合哪些學生](" + h.siteURL("/article/who-is-special-admission-for/") + "): 認識可能適合特殊選才的經歷與才能。\n")
	content.WriteString("- [常見問題](" + h.siteURL("/faq/") + "): 特殊選才準備與申請常見問題。\n\n")
	content.WriteString("- [招生資料回報](" + h.siteURL("/report-data/") + "): 回報各校招生人數或提供歷屆簡章。\n\n")
	content.WriteString("## 資料使用說明\n\n")
	content.WriteString("- 校系招生資料由 S.T.A 整理公開資訊；招生條件、時程及名額可能更新，申請前請以各校當年度正式招生簡章為準。\n")
	content.WriteString("- 各校官方招生網站與簡章 PDF 的連結列於對應校系資料頁。\n\n")
	content.WriteString("## 公開校系招生資料\n\n")
	if len(programs) == 0 {
		content.WriteString("目前沒有可列出的公開校系資料。\n")
	} else {
		for _, program := range programs {
			label := fmt.Sprintf("%d 學年度｜%s｜%s", program.AcademicYear, program.SchoolName, program.AdmissionProgramName)
			label = strings.ReplaceAll(label, "\n", " ")
			label = strings.ReplaceAll(label, "[", "\\[")
			label = strings.ReplaceAll(label, "]", "\\]")
			fmt.Fprintf(&content, "- [%s](%s)\n", label, h.programURL(program.ProgramIdentifier))
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(content.String()))
}

func (h *Handler) allPublishedPrograms(r *http.Request) ([]Program, error) {
	const pageSize = 100
	programs := make([]Program, 0, pageSize)
	for offset := 0; ; offset += pageSize {
		if offset > 10_000 {
			return nil, errors.New("published admissions catalog exceeds sitemap pagination limit")
		}
		page, err := h.repository.ListPrograms(r.Context(), ProgramQuery{Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		programs = append(programs, page...)
		if len(page) < pageSize {
			return programs, nil
		}
	}
}

func (h *Handler) siteURL(path string) string {
	return h.publicBaseURL + path
}

func (h *Handler) programURL(identifier string) string {
	return h.siteURL("/bochures/program/" + url.PathEscape(identifier) + "/")
}

func makeSitemapItems(locations []string) []sitemapURLItem {
	items := make([]sitemapURLItem, len(locations))
	for index, location := range locations {
		items[index] = sitemapURLItem{Location: location}
	}
	return items
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

func (h *Handler) searchPrograms(w http.ResponseWriter, r *http.Request) {
	query, err := parseBrochureSearchQuery(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_query", "admission search query is invalid")
		return
	}
	repository, ok := h.repository.(BrochureSearchRepository)
	if !ok {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	programs, count, err := repository.SearchBrochurePrograms(r.Context(), query)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{
		"data": programs,
		"meta": map[string]any{"limit": query.Limit, "offset": query.Offset, "count": count},
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
	programCode := strings.TrimSpace(r.URL.Query().Get("program_code"))
	if programCode != "" && !codePattern.MatchString(programCode) {
		return ProgramQuery{}, ErrInvalidProgram
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 100 {
		return ProgramQuery{}, ErrInvalidProgram
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 500 {
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
	return ProgramQuery{AcademicYear: year, SchoolCode: schoolCode, ProgramCode: programCode, Search: search, Limit: limit, Offset: offset}, nil
}

func parseBrochureSearchQuery(r *http.Request) (BrochureSearchQuery, error) {
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 100 {
		return BrochureSearchQuery{}, ErrInvalidProgram
	}
	parseFilter := func(key string) (BrochureSearchFilter, error) {
		value := BrochureSearchFilter(strings.TrimSpace(r.URL.Query().Get(key)))
		if value != BrochureSearchAny && value != BrochureSearchRequired && value != BrochureSearchNotRequired {
			return BrochureSearchAny, ErrInvalidProgram
		}
		return value, nil
	}
	skillsTest, err := parseFilter("skills_test")
	if err != nil {
		return BrochureSearchQuery{}, err
	}
	interview, err := parseFilter("interview")
	if err != nil {
		return BrochureSearchQuery{}, err
	}
	portfolio, err := parseFilter("portfolio")
	if err != nil {
		return BrochureSearchQuery{}, err
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return BrochureSearchQuery{}, ErrInvalidProgram
		}
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 10_000 {
			return BrochureSearchQuery{}, ErrInvalidProgram
		}
	}
	return BrochureSearchQuery{
		Search: search, SkillsTest: skillsTest, Interview: interview, Portfolio: portfolio,
		Limit: limit, Offset: offset,
	}, nil
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
