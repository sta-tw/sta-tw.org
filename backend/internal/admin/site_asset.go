package admin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	articleOverviewAssetKey        = "article_overview"
	maxArticleOverviewImageBytes   = int64(5 << 20)
	maxArticleOverviewImageWidth   = 6000
	maxArticleOverviewImageHeight  = 3000
	articleOverviewMultipartBuffer = int64(1 << 20)
)

var (
	errArticleOverviewImageTooLarge = errors.New("article overview image is too large")
	errInvalidArticleOverviewImage  = errors.New("article overview image is invalid")
)

type siteAssetMetadata struct {
	Exists      bool       `json:"exists"`
	ContentType string     `json:"content_type,omitempty"`
	SizeBytes   int64      `json:"size_bytes,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

func (h *Handler) getArticleOverviewImage(w http.ResponseWriter, r *http.Request) {
	var contentType string
	var content []byte
	err := h.pool.QueryRow(r.Context(), `
		SELECT content_type, content
		FROM site_assets
		WHERE asset_key = $1
	`, articleOverviewAssetKey).Scan(&contentType, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "article overview image not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *Handler) getArticleOverviewImageMeta(w http.ResponseWriter, r *http.Request) {
	metadata, err := h.readArticleOverviewImageMeta(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": metadata})
}

func (h *Handler) getAdminArticleOverviewImageMeta(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	metadata, err := h.readArticleOverviewImageMeta(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": metadata})
}

func (h *Handler) readArticleOverviewImageMeta(ctx context.Context) (siteAssetMetadata, error) {
	var metadata siteAssetMetadata
	var contentType string
	var sizeBytes int64
	var updatedAt time.Time
	err := h.pool.QueryRow(ctx, `
		SELECT content_type, octet_length(content)::bigint, updated_at
		FROM site_assets
		WHERE asset_key = $1
	`, articleOverviewAssetKey).Scan(&contentType, &sizeBytes, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return metadata, nil
	}
	if err != nil {
		return metadata, err
	}
	metadata.Exists = true
	metadata.ContentType = contentType
	metadata.SizeBytes = sizeBytes
	metadata.UpdatedAt = &updatedAt
	return metadata, nil
}

func (h *Handler) uploadArticleOverviewImage(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}

	content, contentType, err := readArticleOverviewImage(w, r)
	if errors.Is(err, errArticleOverviewImageTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "image_too_large", "image must be 5 MB or smaller")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_image", "upload a valid PNG or JPEG image")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	defer tx.Rollback(r.Context())

	var beforeContentType string
	var beforeSizeBytes int64
	err = tx.QueryRow(r.Context(), `
		SELECT content_type, octet_length(content)::bigint
		FROM site_assets
		WHERE asset_key = $1
		FOR UPDATE
	`, articleOverviewAssetKey).Scan(&beforeContentType, &beforeSizeBytes)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO site_assets (asset_key, content_type, content, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (asset_key) DO UPDATE SET
			content_type = EXCLUDED.content_type,
			content = EXCLUDED.content,
			updated_at = CURRENT_TIMESTAMP,
			updated_by = EXCLUDED.updated_by
	`, articleOverviewAssetKey, contentType, content, session.Session.Account.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO audit_log (
			actor_account_id, action, entity_type, entity_key, before_data, after_data, reason
		)
		VALUES (
			$1,
			'site_asset.updated',
			'site_asset',
			$2,
			CASE WHEN $3 = '' THEN NULL ELSE jsonb_build_object('content_type', $3, 'size_bytes', $4) END,
			jsonb_build_object('content_type', $5, 'size_bytes', $6),
			'Updated from the admin article overview image editor'
		)
	`, session.Session.Account.ID, articleOverviewAssetKey, beforeContentType, beforeSizeBytes, contentType, int64(len(content))); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	updatedAt := time.Now().UTC()
	writeJSON(w, http.StatusOK, map[string]any{"data": siteAssetMetadata{
		Exists:      true,
		ContentType: contentType,
		SizeBytes:   int64(len(content)),
		UpdatedAt:   &updatedAt,
	}})
}

func (h *Handler) deleteArticleOverviewImage(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	defer tx.Rollback(r.Context())

	var beforeContentType string
	var beforeSizeBytes int64
	err = tx.QueryRow(r.Context(), `
		DELETE FROM site_assets
		WHERE asset_key = $1
		RETURNING content_type, octet_length(content)::bigint
	`, articleOverviewAssetKey).Scan(&beforeContentType, &beforeSizeBytes)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "article overview image is already using the default")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO audit_log (
			actor_account_id, action, entity_type, entity_key, before_data, after_data, reason
		)
		VALUES (
			$1,
			'site_asset.reset',
			'site_asset',
			$2,
			jsonb_build_object('content_type', $3, 'size_bytes', $4),
			NULL,
			'Reset from the admin article overview image editor'
		)
	`, session.Session.Account.ID, articleOverviewAssetKey, beforeContentType, beforeSizeBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func readArticleOverviewImage(w http.ResponseWriter, r *http.Request) ([]byte, string, error) {
	maxRequestBytes := maxArticleOverviewImageBytes + articleOverviewMultipartBuffer
	if r.ContentLength > maxRequestBytes {
		return nil, "", errArticleOverviewImageTooLarge
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return nil, "", errInvalidArticleOverviewImage
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, "", errInvalidArticleOverviewImage
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxArticleOverviewImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%w: could not read upload", errInvalidArticleOverviewImage)
	}
	if int64(len(content)) > maxArticleOverviewImageBytes {
		return nil, "", errArticleOverviewImageTooLarge
	}
	if len(content) == 0 {
		return nil, "", errInvalidArticleOverviewImage
	}

	contentType := http.DetectContentType(content[:minInt(len(content), 512)])
	if contentType != "image/png" && contentType != "image/jpeg" {
		return nil, "", errInvalidArticleOverviewImage
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || (format != "png" && format != "jpeg") ||
		config.Width < 1 || config.Height < 1 ||
		config.Width > maxArticleOverviewImageWidth || config.Height > maxArticleOverviewImageHeight {
		return nil, "", errInvalidArticleOverviewImage
	}
	return content, contentType, nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
