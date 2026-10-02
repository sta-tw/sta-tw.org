package admissions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func TestStampDownloadMetadata(t *testing.T) {
	original := buildMinimalPDF(t, 1)

	stamped, err := stampDownloadMetadata(original, "203.0.113.7")
	if err != nil {
		t.Fatalf("stampDownloadMetadata: %v", err)
	}

	// Author is a recognized standard Info dict key, so pdfcpu surfaces it on
	// XRefTable.Author (populated during validation) rather than through the
	// custom-properties map api.Properties() returns.
	ctx, err := api.ReadValidateAndOptimize(t.Context(), bytes.NewReader(stamped), model.NewStatelessConfiguration(), nil)
	if err != nil {
		t.Fatalf("re-read stamped pdf: %v", err)
	}
	if ctx.Author != "S.T.A 工作團隊" {
		t.Errorf("Author = %q, want S.T.A 工作團隊", ctx.Author)
	}

	props, err := api.Properties(t.Context(), bytes.NewReader(stamped), model.NewStatelessConfiguration())
	if err != nil {
		t.Fatalf("read back properties: %v", err)
	}
	if props["X-STA-Source-IP"] != "203.0.113.7" {
		t.Errorf("X-STA-Source-IP = %q, want 203.0.113.7", props["X-STA-Source-IP"])
	}
	if !strings.Contains(props["X-STA-Notice"], "brochure@mail.sta-tw.org") {
		t.Errorf("X-STA-Notice = %q, missing contact email", props["X-STA-Notice"])
	}
}

func TestStampDownloadMetadataEmptyIP(t *testing.T) {
	original := buildMinimalPDF(t, 1)

	stamped, err := stampDownloadMetadata(original, "")
	if err != nil {
		t.Fatalf("stampDownloadMetadata: %v", err)
	}
	props, err := api.Properties(t.Context(), bytes.NewReader(stamped), model.NewStatelessConfiguration())
	if err != nil {
		t.Fatalf("read back properties: %v", err)
	}
	if props["X-STA-Source-IP"] != "unknown" {
		t.Errorf("X-STA-Source-IP = %q, want unknown fallback", props["X-STA-Source-IP"])
	}
}
