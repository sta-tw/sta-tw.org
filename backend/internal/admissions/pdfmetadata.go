package admissions

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

const brochureContactNotice = "Before using this PDF, please contact us regarding this admissions brochure: brochure@mail.sta-tw.org"

// stampDownloadMetadata rewrites only the Info dictionary (Author plus two
// custom keys) on every public download, so a leaked copy still identifies
// its source even if the hidden OCG layer in applyHiddenWatermark is somehow
// stripped. pdfcpu has no incremental "patch these bytes in place" path —
// AddProperties still does a full read/optimize/write pass — but it never
// touches page content streams, so this is the lightest-weight PDF mutation
// this library offers short of not touching the file at all.
func stampDownloadMetadata(pdfBytes []byte, requesterIP string) ([]byte, error) {
	if requesterIP == "" {
		requesterIP = "unknown"
	}
	conf := model.NewStatelessConfiguration()
	properties := map[string]string{
		"Author":          "S.T.A 工作團隊",
		"X-STA-Source-IP": requesterIP,
		"X-STA-Notice":    brochureContactNotice,
	}
	var out bytes.Buffer
	if err := api.AddProperties(context.Background(), bytes.NewReader(pdfBytes), &out, properties, conf); err != nil {
		return nil, fmt.Errorf("stamp download metadata: %w", err)
	}
	return out.Bytes(), nil
}
