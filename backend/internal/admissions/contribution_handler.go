package admissions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/email"
	"sta-backend/internal/emailinquiries"
	"sta-backend/internal/security"
	"sta-backend/internal/storage"
)

const (
	maxContributionFileBytes int64 = 10 << 20
	maxContributionFiles           = 5
	maxContributionFormBytes       = maxContributionFileBytes*maxContributionFiles + (2 << 20)
)

type ContributionHandler struct {
	authService        *auth.Service
	repository         *PostgresRepository
	fieldCipher        *auth.FieldCipher
	blobStore          storage.BlobStore
	scanner            storage.Scanner
	emailQueue         ContributionEmailQueue
	replyMailer        email.Sender
	replyTemplate      string
	forumNotifier      ContributionForumNotifier
	forumChannelID     string
	forumRoleIDs       []string
	logger             *slog.Logger
	turnstile          *auth.TurnstileVerifier
	formLimiter        *security.FixedWindowLimiter
	distributedLimiter security.DistributedLimiter
}

type ContributionEmailQueue interface {
	EnqueueEmailAddress(ctx context.Context, recipient, dedupKey, subject, text string) error
}

type ContributionForumNotifier interface {
	CreateContributionThread(ctx context.Context, forumChannelID, subject, body string, mentionRoleIDs []string) (string, error)
}

type ContributionForumReplyNotifier interface {
	PostContributionReply(ctx context.Context, threadID, staffName, body string) error
}

func NewContributionHandler(
	authService *auth.Service,
	repository *PostgresRepository,
	fieldCipher *auth.FieldCipher,
	blobStore storage.BlobStore,
	scanner storage.Scanner,
) (*ContributionHandler, error) {
	if authService == nil || repository == nil || fieldCipher == nil {
		return nil, errors.New("admissions contribution handler dependencies are missing")
	}
	return &ContributionHandler{
		authService: authService,
		repository:  repository,
		fieldCipher: fieldCipher,
		blobStore:   blobStore,
		scanner:     scanner,
		logger:      slog.Default(),
		formLimiter: security.NewFixedWindowLimiter(10, time.Hour, 10000),
	}, nil
}

func (h *ContributionHandler) ConfigureEmailQueue(queue ContributionEmailQueue) {
	h.emailQueue = queue
}

func (h *ContributionHandler) ConfigureReplyMailer(mailer email.Sender, replyTemplate *string) {
	h.replyMailer = mailer
	h.replyTemplate = emailinquiries.DefaultReplyTemplate()
	if replyTemplate != nil && strings.TrimSpace(*replyTemplate) != "" {
		h.replyTemplate = *replyTemplate
	}
}

func (h *ContributionHandler) ConfigureForumNotifier(notifier ContributionForumNotifier, channelID string, roleIDs []string) {
	h.forumNotifier = notifier
	h.forumChannelID = strings.TrimSpace(channelID)
	h.forumRoleIDs = append([]string(nil), roleIDs...)
}

func (h *ContributionHandler) ConfigureLogger(logger *slog.Logger) {
	if logger != nil {
		h.logger = logger
	}
}

func (h *ContributionHandler) ConfigureDistributedLimiter(limiter security.DistributedLimiter) {
	h.distributedLimiter = limiter
}

func (h *ContributionHandler) ConfigureTurnstile(verifier *auth.TurnstileVerifier) {
	h.turnstile = verifier
}

func (h *ContributionHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/admissions/contributions", h.create)
	mux.HandleFunc("GET /api/v1/admissions/contributors", h.listPublicContributors)
	mux.HandleFunc("GET /api/v1/admin/admissions/contributions", h.listAdmin)
	mux.HandleFunc("POST /api/v1/admin/admissions/contributions/{id}/review", h.reviewAdmin)
	mux.HandleFunc("GET /api/v1/admin/admissions/contributions/{id}/content", h.contentAdmin)
	mux.HandleFunc("GET /api/v1/admin/admissions/contributions/{id}/attachments/{attachmentID}/content", h.attachmentContentAdmin)
}

func (h *ContributionHandler) create(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	ip := contributionClientIP(r)
	result := h.formLimiter.Take(ip, now)
	security.WriteRateLimitHeaders(w, result, now)
	allowed, err := security.CheckDistributed(r.Context(), h.distributedLimiter, result.Allowed,
		"admissions-contribution-form", ip, 10, time.Hour, now)
	if err != nil {
		writeAdmissionError(w, http.StatusServiceUnavailable, "rate_limit_unavailable", "submission is temporarily unavailable")
		return
	}
	if !allowed {
		writeAdmissionError(w, http.StatusTooManyRequests, "rate_limited", "too many submissions from this address")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxContributionFormBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_multipart", "submission form is invalid")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	// A quiet honeypot keeps simple form bots from filling the review queue.
	if strings.TrimSpace(r.FormValue("website")) != "" {
		writeAdmissionJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"received": true}})
		return
	}
	if h.turnstile != nil {
		if err := h.turnstile.Verify(r.Context(), strings.TrimSpace(r.FormValue("turnstile_token")), "admissions_contribution", ip); err != nil {
			if errors.Is(err, auth.ErrTurnstileInvalid) {
				writeAdmissionError(w, http.StatusForbidden, "turnstile_invalid", "verification failed")
			} else {
				writeAdmissionError(w, http.StatusServiceUnavailable, "turnstile_unavailable", "verification service is temporarily unavailable")
			}
			return
		}
	}

	academicYear, err := strconv.Atoi(strings.TrimSpace(r.FormValue("academic_year")))
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_contribution", "submission data is invalid")
		return
	}
	files := submittedContributionFiles(r.MultipartForm.File["file"])
	if len(files) > maxContributionFiles {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_file", "at most five files can be submitted")
		return
	}
	input := ContributionInput{
		Email:           r.FormValue("email"),
		AcademicYear:    academicYear,
		Category:        strings.TrimSpace(r.FormValue("category")),
		SchoolName:      r.FormValue("school_name"),
		ReferenceURL:    r.FormValue("reference_url"),
		Nickname:        r.FormValue("nickname"),
		PublishNickname: r.FormValue("publish_nickname") == "true",
	}
	if input.Category == ContributionApplicants || input.Category == ContributionSecond || input.Category == ContributionWaitlist {
		if len(files) > 0 {
			if len(r.MultipartForm.Value["program_name"]) > 0 || len(r.MultipartForm.Value["people_count"]) > 0 || strings.TrimSpace(r.FormValue("data_text")) != "" {
				writeAdmissionError(w, http.StatusBadRequest, "invalid_contribution", "choose manual data entry or file upload")
				return
			}
		} else if input.ProgramRows, err = contributionRowsFromRequest(r); err != nil {
			writeAdmissionError(w, http.StatusBadRequest, "invalid_contribution", "submission data is invalid")
			return
		}
	} else if input.Category == ContributionBrochure && (len(r.MultipartForm.Value["program_name"]) > 0 || len(r.MultipartForm.Value["people_count"]) > 0 || strings.TrimSpace(r.FormValue("data_text")) != "") {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_contribution", "submission data is invalid")
		return
	}
	input.Email = auth.NormalizeEmail(input.Email)
	if input.Category == ContributionBrochure && len(files) > 1 {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_file", "a historical brochure submission can include one PDF")
		return
	}
	if input.NormalizeAndValidate(len(files) > 0) != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_contribution", "submission data is invalid")
		return
	}

	stagedUploads := make([]storage.StagedUpload, 0, len(files))
	defer func() {
		for _, upload := range stagedUploads {
			upload.Close()
		}
	}()
	if len(files) > 0 {
		if h.blobStore == nil {
			writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "file uploads are temporarily unavailable")
			return
		}
		if h.scanner == nil {
			writeAdmissionError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
			return
		}
		for _, header := range files {
			file, err := header.Open()
			if err != nil {
				writeAdmissionError(w, http.StatusBadRequest, "invalid_file", "an uploaded file could not be read")
				return
			}
			staged, stageErr := storage.StageUploadWithLimit(header.Filename, header.Header.Get("Content-Type"), file, maxContributionFileBytes)
			_ = file.Close()
			if stageErr != nil || (input.Category == ContributionBrochure && staged.ContentType != "application/pdf") {
				staged.Close()
				writeAdmissionError(w, http.StatusBadRequest, "invalid_file", "file type is not supported or exceeds 10 MB")
				return
			}
			if err := storage.ScanStagedUpload(r.Context(), h.scanner, staged); err != nil {
				staged.Close()
				if errors.Is(err, storage.ErrMalwareDetected) {
					writeAdmissionError(w, http.StatusUnprocessableEntity, "malware_detected", "uploaded file was rejected")
				} else {
					writeAdmissionError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
				}
				return
			}
			stagedUploads = append(stagedUploads, staged)
		}
	}

	fileRecords := make([]ContributionFile, 0, len(stagedUploads))
	storedKeys := make([]string, 0, len(stagedUploads))
	cleanupStoredFiles := true
	defer func() {
		if cleanupStoredFiles && h.blobStore != nil {
			for _, key := range storedKeys {
				_ = h.blobStore.Remove(context.Background(), key)
			}
		}
	}()
	for _, upload := range stagedUploads {
		fileID, err := uuid.NewRandom()
		if err != nil {
			writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		extension := strings.ToLower(filepath.Ext(upload.OriginalName))
		storageKey := "admissions/contributions/" + fileID.String() + extension
		fileRecords = append(fileRecords, ContributionFile{
			StorageKey: storageKey,
			Filename:   upload.OriginalName,
			MIMEType:   upload.ContentType,
			SizeBytes:  upload.Size,
			SHA256:     upload.SHA256Hex,
		})
	}
	input.Files = fileRecords
	if input.NormalizeAndValidate(len(files) > 0) != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_contribution", "submission data is invalid")
		return
	}
	for index, upload := range stagedUploads {
		storageKey := fileRecords[index].StorageKey
		stagedFile, err := os.Open(upload.Path)
		if err != nil {
			writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		putErr := h.blobStore.Put(r.Context(), storageKey, stagedFile, upload.Size, upload.ContentType)
		_ = stagedFile.Close()
		if putErr != nil {
			writeAdmissionError(w, http.StatusBadGateway, "storage_unavailable", "file storage is temporarily unavailable")
			return
		}
		storedKeys = append(storedKeys, storageKey)
	}

	emailCiphertext, err := h.fieldCipher.Seal(input.Email)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	receipt, err := h.repository.CreateContribution(r.Context(), input, emailCiphertext, fileRecords)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "submission could not be saved")
		return
	}
	cleanupStoredFiles = false
	receipt.EmailQueued = h.queueContributionReceipt(input, receipt, fileRecords)
	receipt.ForumPosted = h.postContributionToForum(input, receipt, fileRecords)
	writeAdmissionJSON(w, http.StatusCreated, map[string]any{"data": receipt})
}

func (h *ContributionHandler) queueContributionReceipt(input ContributionInput, receipt ContributionReceipt, files []ContributionFile) bool {
	if h.emailQueue == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subject := contributionReceiptSubject(receipt.ID)
	body := contributionNotificationBody(input, receipt, files)
	if err := h.emailQueue.EnqueueEmailAddress(ctx, input.Email, "admissions-contribution:"+receipt.ID.String(), subject, body); err != nil {
		h.logger.Warn("admissions contribution receipt could not be queued", "contribution_id", receipt.ID, "error", err)
		return false
	}
	return true
}

func (h *ContributionHandler) postContributionToForum(input ContributionInput, receipt ContributionReceipt, files []ContributionFile) bool {
	if h.forumNotifier == nil || h.forumChannelID == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	threadID, err := h.forumNotifier.CreateContributionThread(
		ctx,
		h.forumChannelID,
		contributionThreadTitle(input, receipt),
		contributionNotificationBody(input, receipt, files),
		h.forumRoleIDs,
	)
	if err != nil {
		h.logger.Warn("admissions contribution forum post failed", "contribution_id", receipt.ID, "error", err)
		return false
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}
	if err := h.repository.SetContributionDiscordThread(ctx, receipt.ID, threadID); err != nil {
		h.logger.Warn("admissions contribution forum thread could not be linked", "contribution_id", receipt.ID, "error", err)
		return false
	}
	return true
}

// ReplyByDiscord sends a manually authored email from /re in a contribution's
// Discord forum thread. The forum thread ID is looked up against a stored
// contribution before its encrypted contact email is used.
func (h *ContributionHandler) ReplyByDiscord(ctx context.Context, _ string, threadID, staffName, staffDiscordID, body, signature string) error {
	body = strings.TrimSpace(body)
	signature = strings.TrimSpace(signature)
	if body == "" || signature == "" {
		return ErrContributionReplyInvalid
	}
	if h.replyMailer == nil {
		return errors.New("admissions contribution reply mailer is not configured")
	}
	target, err := h.repository.FindContributionReplyTarget(ctx, threadID)
	if err != nil {
		return err
	}
	to, err := h.fieldCipher.Open(target.EmailCiphertext)
	if err != nil {
		return fmt.Errorf("decrypt admissions contribution reply address: %w", err)
	}
	subject := contributionReceiptSubject(target.ID)
	fullBody := emailinquiries.RenderReplyBody(h.replyTemplate, emailinquiries.ReplyTemplateVars{
		Body: body, Signature: signature, Subject: subject, StaffName: staffName,
		SenderEmail: to, Date: time.Now().Format("2006年01月02日"),
	})
	if err := h.replyMailer.Send(ctx, email.Message{
		To: to, Subject: "Re: " + subject, Text: fullBody,
		Bcc: []string{email.SupportArchiveBCCAddress},
	}); err != nil {
		return err
	}
	if notifier, ok := h.forumNotifier.(ContributionForumReplyNotifier); ok {
		forumBody := body + "\n\n" + signature
		if err := notifier.PostContributionReply(ctx, threadID, staffName, forumBody); err != nil {
			h.logger.Warn("admissions contribution reply confirmation could not be posted", "contribution_id", target.ID, "staff_discord_id", staffDiscordID, "error", err)
		}
	}
	return nil
}

func contributionReceiptSubject(id uuid.UUID) string {
	return fmt.Sprintf("特殊選才資料回報副本（%s）", id.String()[:8])
}

func contributionNotificationBody(input ContributionInput, receipt ContributionReceipt, files []ContributionFile) string {
	category := map[string]string{
		ContributionApplicants: "應屆報名人數",
		ContributionSecond:     "進入二階人數",
		ContributionWaitlist:   "應屆遞補人數",
		ContributionBrochure:   "歷屆簡章",
	}[input.Category]
	var body strings.Builder
	fmt.Fprintf(&body, "回報編號：%s\n學年度：%d\n類別：%s\n", receipt.ID, input.AcademicYear, category)
	if input.SchoolName != "" {
		fmt.Fprintf(&body, "學校：%s\n", input.SchoolName)
	}
	if input.DataText != "" {
		fmt.Fprintf(&body, "\n回報內容：\n%s\n", input.DataText)
	}
	if input.ReferenceURL != "" {
		fmt.Fprintf(&body, "\n參考連結：%s\n", input.ReferenceURL)
	}
	if len(files) > 0 {
		body.WriteString("\n上傳檔案：\n")
		for _, file := range files {
			fmt.Fprintf(&body, "- %s\n", file.Filename)
		}
	}
	return body.String()
}

func contributionThreadTitle(input ContributionInput, receipt ContributionReceipt) string {
	name := strings.Join(strings.Fields(input.SchoolName), " ")
	if name == "" {
		name = "資料回報"
	}
	title := fmt.Sprintf("資料回報｜%d｜%s｜%s", input.AcademicYear, name, receipt.ID.String()[:8])
	return truncateUTF8Bytes(title, 100)
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.ValidString(value[:maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}

func (h *ContributionHandler) listPublicContributors(w http.ResponseWriter, r *http.Request) {
	items, err := h.repository.ListPublicContributors(r.Context())
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60, s-maxage=60")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *ContributionHandler) listAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmissionsAdmin(w, r, false); !ok {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != ContributionPending && status != ContributionAccepted && status != ContributionRejected {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_status", "contribution status is invalid")
		return
	}
	limit, offset, err := contributionPage(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_pagination", "contribution pagination is invalid")
		return
	}
	items, err := h.repository.ListContributions(r.Context(), status, limit, offset)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	for index := range items {
		items[index].Email, err = h.fieldCipher.Open(items[index].EmailCiphertext)
		if err != nil {
			writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		items[index].EmailCiphertext = nil
		items[index].DataText = normalizeContributionDataText(items[index].DataText)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"limit": limit, "offset": offset, "count": len(items)},
	})
}

func (h *ContributionHandler) reviewAdmin(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmissionsAdmin(w, r, true)
	if !ok {
		return
	}
	contributionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_id", "contribution id is invalid")
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := decodeBrochureJSON(r, &input); err != nil || (input.Status != ContributionAccepted && input.Status != ContributionRejected) {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_status", "status must be accepted or rejected")
		return
	}
	item, err := h.repository.ReviewContribution(r.Context(), session.Session.Account.ID, contributionID, input.Status)
	if err != nil {
		switch {
		case errors.Is(err, ErrContributionNotFound):
			writeAdmissionError(w, http.StatusNotFound, "not_found", "contribution was not found")
		case errors.Is(err, ErrContributionInvalidStatus):
			writeAdmissionError(w, http.StatusConflict, "invalid_status", "contribution has already been reviewed")
		default:
			writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	item.Email, err = h.fieldCipher.Open(item.EmailCiphertext)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	item.EmailCiphertext = nil
	item.DataText = normalizeContributionDataText(item.DataText)
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": item})
}

func contributionRowsFromRequest(r *http.Request) ([]ContributionProgramRow, error) {
	programNames := r.MultipartForm.Value["program_name"]
	peopleCounts := r.MultipartForm.Value["people_count"]
	legacyText := strings.TrimSpace(r.FormValue("data_text"))
	if len(programNames) == 0 && len(peopleCounts) == 0 {
		return parseContributionRowsText(legacyText)
	}
	if legacyText != "" || len(programNames) == 0 || len(programNames) != len(peopleCounts) || len(programNames) > maxContributionRows {
		return nil, ErrInvalidContribution
	}
	rows := make([]ContributionProgramRow, 0, len(programNames))
	for index, programName := range programNames {
		countText := strings.TrimSpace(peopleCounts[index])
		if countText == "" {
			return nil, ErrInvalidContribution
		}
		for _, digit := range countText {
			if digit < '0' || digit > '9' {
				return nil, ErrInvalidContribution
			}
		}
		people, err := strconv.Atoi(countText)
		if err != nil || people > 1_000_000_000 {
			return nil, ErrInvalidContribution
		}
		rows = append(rows, ContributionProgramRow{ProgramName: programName, People: people})
	}
	return rows, nil
}

func (h *ContributionHandler) contentAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmissionsAdmin(w, r, false); !ok {
		return
	}
	if h.blobStore == nil {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "file storage is unavailable")
		return
	}
	reader, ok := h.blobStore.(storage.BlobReader)
	if !ok {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "PDF content is unavailable")
		return
	}
	contributionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_id", "contribution id is invalid")
		return
	}
	storageKey, err := h.repository.ContributionStorageKey(r.Context(), contributionID)
	if err != nil {
		if errors.Is(err, ErrContributionNotFound) {
			writeAdmissionError(w, http.StatusNotFound, "not_found", "contribution file was not found")
		} else {
			writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	object, err := reader.Get(r.Context(), storageKey)
	if err != nil {
		writeAdmissionError(w, http.StatusBadGateway, "storage_unavailable", "file storage is unavailable")
		return
	}
	defer object.Close()
	if object.Size <= 0 || object.Size > storage.MaxPortfolioFileBytes {
		writeAdmissionError(w, http.StatusBadGateway, "invalid_file", "stored PDF is invalid")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="admissions-contribution.pdf"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	_, _ = io.Copy(w, object)
}

func (h *ContributionHandler) attachmentContentAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmissionsAdmin(w, r, false); !ok {
		return
	}
	contributionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_id", "contribution id is invalid")
		return
	}
	attachmentID, err := uuid.Parse(r.PathValue("attachmentID"))
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_id", "attachment id is invalid")
		return
	}
	attachment, err := h.repository.ContributionAttachment(r.Context(), contributionID, attachmentID)
	if err != nil {
		if errors.Is(err, ErrContributionNotFound) {
			writeAdmissionError(w, http.StatusNotFound, "not_found", "contribution attachment was not found")
		} else {
			writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	if h.blobStore == nil {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "file storage is unavailable")
		return
	}
	reader, ok := h.blobStore.(storage.BlobReader)
	if !ok {
		writeAdmissionError(w, http.StatusServiceUnavailable, "storage_unavailable", "file content is unavailable")
		return
	}
	object, err := reader.Get(r.Context(), attachment.StorageKey)
	if err != nil {
		writeAdmissionError(w, http.StatusBadGateway, "storage_unavailable", "file storage is unavailable")
		return
	}
	defer object.Close()
	if object.Size <= 0 || object.Size > storage.MaxPortfolioFileBytes || object.Size != attachment.SizeBytes {
		writeAdmissionError(w, http.StatusBadGateway, "invalid_file", "stored file is invalid")
		return
	}
	disposition := "attachment"
	if attachment.MIMEType == "application/pdf" || strings.HasPrefix(attachment.MIMEType, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", attachment.MIMEType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": attachment.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	_, _ = io.Copy(w, object)
}

func submittedContributionFiles(files []*multipart.FileHeader) []*multipart.FileHeader {
	result := make([]*multipart.FileHeader, 0, len(files))
	for _, file := range files {
		if file == nil || (file.Filename == "" && file.Size == 0) {
			continue
		}
		result = append(result, file)
	}
	return result
}

func (h *ContributionHandler) requireAdmissionsAdmin(w http.ResponseWriter, r *http.Request, mutation bool) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeAdmissionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		} else {
			writeAdmissionError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		}
		return auth.RequestSession{}, false
	}
	canManageAdmissions, err := h.authService.CanManageAdmissions(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !canManageAdmissions {
		writeAdmissionError(w, http.StatusForbidden, "admin_required", "admissions administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeAdmissionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	if mutation {
		if err := h.authService.AuthorizeMutation(r, session); err != nil {
			writeAdmissionError(w, http.StatusForbidden, "csrf_required", "request verification failed")
			return auth.RequestSession{}, false
		}
		if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
			writeAdmissionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
	}
	return session, true
}

func contributionPage(r *http.Request) (int, int, error) {
	limit, offset := 100, 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			return 0, 0, ErrInvalidContribution
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 100000 {
			return 0, 0, ErrInvalidContribution
		}
	}
	return limit, offset, nil
}

func contributionClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		peer := net.ParseIP(host)
		forwarded := net.ParseIP(strings.TrimSpace(r.Header.Get("X-STA-Client-IP")))
		if peer != nil && (peer.IsPrivate() || peer.IsLoopback()) && forwarded != nil {
			return forwarded.String()
		}
	}
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}
