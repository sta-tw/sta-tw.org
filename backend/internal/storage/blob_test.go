package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestStageUploadValidatesPDFAndHashesStreamingData(t *testing.T) {
	payload := []byte("%PDF-1.7\nminimal test payload")
	upload, err := StageUpload("備審資料.pdf", "application/pdf", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("StageUpload() error = %v", err)
	}
	defer upload.Close()
	wantHash := sha256.Sum256(payload)
	if upload.SHA256Hex != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("SHA256Hex = %q, want %q", upload.SHA256Hex, hex.EncodeToString(wantHash[:]))
	}
	if upload.Size != int64(len(payload)) || upload.ContentType != "application/pdf" {
		t.Fatalf("upload metadata = %#v", upload)
	}
	if _, err := os.Stat(upload.Path); err != nil {
		t.Fatalf("staged upload does not exist: %v", err)
	}
}

func TestStageUploadRejectsSignatureMismatchAndPathName(t *testing.T) {
	if _, err := StageUpload("../unsafe.pdf", "application/pdf", bytes.NewReader([]byte("%PDF-1.7"))); err == nil {
		t.Fatal("StageUpload(path traversal) error = nil")
	}
	if _, err := StageUpload("notes.pdf", "application/pdf", bytes.NewReader([]byte("not a PDF"))); err == nil {
		t.Fatal("StageUpload(signature mismatch) error = nil")
	}
}

func TestStageUploadWithLimitRejectsOversizedFile(t *testing.T) {
	if _, err := StageUploadWithLimit("notes.txt", "text/plain", bytes.NewReader([]byte("hello")), 4); err == nil {
		t.Fatal("StageUploadWithLimit() error = nil, want size error")
	}
}
