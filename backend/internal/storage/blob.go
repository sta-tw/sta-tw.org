package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const MaxPortfolioFileBytes int64 = 50 << 20

type BlobStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Remove(context.Context, string) error
	PresignGet(context.Context, string, time.Duration) (*url.URL, error)
}

type MinioStore struct {
	client *minio.Client
	bucket string
}

func NewMinioStore(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinioStore, error) {
	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, errors.New("object storage is not fully configured")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create object storage client: %w", err)
	}
	return &MinioStore{client: client, bucket: bucket}, nil
}

// Ping verifies the configured bucket is reachable. Suitable for a readiness
// probe.
func (s *MinioStore) Ping(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("reach object storage: %w", err)
	}
	if !exists {
		return fmt.Errorf("object storage bucket %q does not exist", s.bucket)
	}
	return nil
}

func (s *MinioStore) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if err := validateStorageKey(key); err != nil {
		return err
	}
	if size <= 0 || size > MaxPortfolioFileBytes {
		return errors.New("object size is invalid")
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

func (s *MinioStore) Remove(ctx context.Context, key string) error {
	if err := validateStorageKey(key); err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object: %w", err)
	}
	return nil
}

func (s *MinioStore) PresignGet(ctx context.Context, key string, expiry time.Duration) (*url.URL, error) {
	if err := validateStorageKey(key); err != nil {
		return nil, err
	}
	if expiry <= 0 || expiry > 15*time.Minute {
		expiry = 5 * time.Minute
	}
	result, err := s.client.PresignedGetObject(ctx, s.bucket, key, expiry, nil)
	if err != nil {
		return nil, fmt.Errorf("presign object: %w", err)
	}
	return result, nil
}

func validateStorageKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.ContainsRune(key, '\\') {
		return errors.New("invalid object storage key")
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("invalid object storage key")
		}
	}
	return nil
}

type StagedUpload struct {
	Path         string
	OriginalName string
	ContentType  string
	Size         int64
	SHA256Hex    string
}

func (u StagedUpload) Close() {
	if u.Path != "" {
		_ = os.Remove(u.Path)
	}
}

func StageUpload(originalName, declaredContentType string, source io.Reader) (StagedUpload, error) {
	return StageUploadWithLimit(originalName, declaredContentType, source, MaxPortfolioFileBytes)
}

func StageUploadWithLimit(originalName, declaredContentType string, source io.Reader, maxBytes int64) (StagedUpload, error) {
	if source == nil || maxBytes <= 0 || maxBytes > MaxPortfolioFileBytes {
		return StagedUpload{}, errors.New("upload limit is invalid")
	}
	name, err := sanitizeFileName(originalName)
	if err != nil {
		return StagedUpload{}, err
	}
	temp, err := os.CreateTemp("", "sta-upload-*")
	if err != nil {
		return StagedUpload{}, fmt.Errorf("create upload staging file: %w", err)
	}
	path := temp.Name()
	removeOnError := true
	defer func() {
		_ = temp.Close()
		if removeOnError {
			_ = os.Remove(path)
		}
	}()

	hasher := sha256.New()
	writer := io.MultiWriter(temp, hasher)
	count, err := io.Copy(writer, io.LimitReader(source, maxBytes+1))
	if err != nil {
		return StagedUpload{}, fmt.Errorf("stage upload: %w", err)
	}
	if count <= 0 || count > maxBytes {
		return StagedUpload{}, errors.New("file size is outside the allowed range")
	}
	if err := temp.Close(); err != nil {
		return StagedUpload{}, fmt.Errorf("close staged upload: %w", err)
	}
	headerFile, err := os.Open(path)
	if err != nil {
		return StagedUpload{}, fmt.Errorf("open staged upload: %w", err)
	}
	header := make([]byte, 512)
	headerSize, readErr := io.ReadFull(headerFile, header)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		_ = headerFile.Close()
		return StagedUpload{}, fmt.Errorf("read staged upload header: %w", readErr)
	}
	_ = headerFile.Close()
	contentType, err := validateFileType(name, declaredContentType, header[:headerSize])
	if err != nil {
		return StagedUpload{}, err
	}
	removeOnError = false
	return StagedUpload{Path: path, OriginalName: name, ContentType: contentType, Size: count, SHA256Hex: hex.EncodeToString(hasher.Sum(nil))}, nil
}

func sanitizeFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\") {
		return "", errors.New("file name is invalid")
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", errors.New("file name contains a control character")
		}
	}
	if filepath.Base(name) != name {
		return "", errors.New("file name is invalid")
	}
	return name, nil
}

func validateFileType(name, declared string, header []byte) (string, error) {
	extension := strings.ToLower(filepath.Ext(name))
	declared = strings.ToLower(strings.TrimSpace(strings.Split(declared, ";")[0]))
	allowed := map[string]string{
		".pdf":  "application/pdf",
		".csv":  "text/csv",
		".tsv":  "text/tab-separated-values",
		".txt":  "text/plain",
		".json": "application/json",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".ppt":  "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
	}
	contentType, ok := allowed[extension]
	if !ok || (declared != "" && declared != contentType && !(strings.HasPrefix(declared, "application/vnd.openxmlformats-officedocument") && strings.HasPrefix(contentType, "application/vnd.openxmlformats-officedocument"))) {
		return "", errors.New("file type is not allowed")
	}
	if !hasExpectedSignature(extension, header) {
		return "", errors.New("file signature does not match extension")
	}
	return contentType, nil
}

func hasExpectedSignature(extension string, header []byte) bool {
	switch extension {
	case ".pdf":
		return len(header) >= 5 && string(header[:5]) == "%PDF-"
	case ".csv", ".tsv", ".txt":
		// Delimited/text lists have no magic number. The upload is still bounded,
		// extension/MIME checked, and passed through the configured malware scan.
		return len(header) > 0
	case ".json":
		trimmed := strings.TrimSpace(string(header))
		return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
	case ".jpg", ".jpeg":
		return len(header) >= 3 && header[0] == 0xff && header[1] == 0xd8 && header[2] == 0xff
	case ".png":
		return len(header) >= 8 && string(header[:8]) == "\x89PNG\r\n\x1a\n"
	case ".doc", ".xls", ".ppt":
		return len(header) >= 8 && string(header[:8]) == "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
	case ".docx", ".xlsx", ".pptx":
		return len(header) >= 2 && header[0] == 'P' && header[1] == 'K'
	default:
		return false
	}
}
