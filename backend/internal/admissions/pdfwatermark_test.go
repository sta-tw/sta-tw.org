package admissions

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// buildMinimalPDF assembles a tiny, uncompressed, hand-rolled multi-page PDF
// (no third-party PDF-writer dependency) purely so this test has something
// realistic to watermark. Offsets are computed from the bytes actually
// written, not hardcoded, so this stays correct if the content changes.
func buildMinimalPDF(t *testing.T, pageCount int) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	catalogObjNr := 1
	pagesObjNr := 2
	firstPageObjNr := 3

	// Reserve object 1 (Catalog) and 2 (Pages) slots; write them after the
	// page objects so Kids can be known, but keep numbering fixed.
	pageObjNrs := make([]int, pageCount)
	for i := 0; i < pageCount; i++ {
		pageObjNrs[i] = firstPageObjNr + i*2 // page obj, then its content stream obj
	}
	contentObjNrs := make([]int, pageCount)
	for i := 0; i < pageCount; i++ {
		contentObjNrs[i] = pageObjNrs[i] + 1
	}

	// Placeholder offsets slice indexed by obj number (1-based).
	totalObjs := 2 + pageCount*2
	objOffsets := make([]int, totalObjs+1)

	kids := make([]string, pageCount)
	for i := range kids {
		kids[i] = fmt.Sprintf("%d 0 R", pageObjNrs[i])
	}

	objOffsets[catalogObjNr] = buf.Len()
	buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Catalog /Pages %d 0 R >>\nendobj\n", catalogObjNr, pagesObjNr))

	objOffsets[pagesObjNr] = buf.Len()
	buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n",
		pagesObjNr, strings.Join(kids, " "), pageCount))

	for i := 0; i < pageCount; i++ {
		content := fmt.Sprintf("BT /F1 12 Tf 50 700 Td (Sample brochure page %d) Tj ET", i+1)
		objOffsets[pageObjNrs[i]] = buf.Len()
		buf.WriteString(fmt.Sprintf(
			"%d 0 obj\n<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] "+
				"/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >> "+
				"/Contents %d 0 R >>\nendobj\n",
			pageObjNrs[i], pagesObjNr, contentObjNrs[i]))

		objOffsets[contentObjNrs[i]] = buf.Len()
		buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n",
			contentObjNrs[i], len(content), content))
	}

	xrefOffset := buf.Len()
	buf.WriteString(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", totalObjs+1))
	for i := 1; i <= totalObjs; i++ {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", objOffsets[i]))
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF",
		totalObjs+1, catalogObjNr, xrefOffset))

	return buf.Bytes()
}

func TestApplyHiddenWatermarkSurvivesPdftotextButStaysInvisible(t *testing.T) {
	original := buildMinimalPDF(t, 4)

	watermarked, err := applyHiddenWatermark(original)
	if err != nil {
		t.Fatalf("applyHiddenWatermark: %v", err)
	}

	// 1. Still a structurally valid PDF.
	if _, err := api.ReadValidateAndOptimize(t.Context(), bytes.NewReader(watermarked), model.NewStatelessConfiguration(), nil); err != nil {
		t.Fatalf("watermarked pdf failed to re-parse: %v", err)
	}

	// 2. The marker is genuinely present in the file's page content streams
	// (ground truth, read back via pdfcpu's own object model — this is what
	// an investigator doing real forensics, not just a casual text dump,
	// would find).
	encoded := base64Encode(sourceMarkerPlain)
	reassembled := extractHiddenFragmentPayload(t, watermarked)
	if reassembled != encoded {
		t.Fatalf("watermark payload mismatch.\nwant: %s\ngot:  %s", encoded, reassembled)
	}

	// 3. A plain `pdftotext` dump — the "casual check" this is meant to
	// survive — does NOT surface it: poppler honors the OCG's default-OFF
	// state and omits marked-content it would not render. The visible page
	// text is unaffected.
	if _, err := exec.LookPath("pdftotext"); err == nil {
		out := runPdftotext(t, watermarked)
		if strings.Contains(out, encoded) {
			t.Errorf("pdftotext surfaced the hidden marker — expected it to stay hidden from plain extraction")
		}
		if !strings.Contains(out, "Sample brochure page 1") {
			t.Errorf("original visible content missing from watermarked pdf")
		}
	} else {
		t.Log("pdftotext not available, skipping extraction check")
	}
}

var hiddenFragmentPattern = regexp.MustCompile(`/OC /MC0 BDC\nBT\n/WMFont 1 Tf\n1 0 0 1 1 1 Tm\n\(([A-Za-z0-9+/=]*)\) Tj`)

// extractHiddenFragmentPayload re-parses watermarked, reads the raw
// (decoded) content stream bytes of every page in document order regardless
// of OCG visibility — i.e. what's actually in the file, not what a viewer
// would show — and reassembles just the hidden fragments' payload.
func extractHiddenFragmentPayload(t *testing.T, watermarked []byte) string {
	all := extractHiddenFragments(t, watermarked)
	var sb strings.Builder
	for _, m := range hiddenFragmentPattern.FindAllStringSubmatch(all, -1) {
		sb.WriteString(m[1])
	}
	return sb.String()
}

func extractHiddenFragments(t *testing.T, watermarked []byte) string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(t.Context(), bytes.NewReader(watermarked), model.NewStatelessConfiguration(), nil)
	if err != nil {
		t.Fatalf("re-read watermarked pdf: %v", err)
	}
	if err := ctx.EnsurePageCount(); err != nil {
		t.Fatalf("page count: %v", err)
	}
	var all strings.Builder
	for pageNr := 1; pageNr <= ctx.PageCount; pageNr++ {
		pageDict, _, _, err := ctx.PageDict(t.Context(), pageNr, false)
		if err != nil {
			t.Fatalf("page dict %d: %v", pageNr, err)
		}
		obj, found := pageDict.Find("Contents")
		if !found {
			continue
		}
		resolved, err := ctx.Dereference(obj)
		if err != nil {
			t.Fatalf("dereference contents %d: %v", pageNr, err)
		}
		refs, ok := resolved.(types.Array)
		if !ok {
			refs = types.Array{obj}
		}
		for _, ref := range refs {
			streamObj, err := ctx.Dereference(ref)
			if err != nil {
				continue
			}
			sd, ok := streamObj.(types.StreamDict)
			if !ok {
				continue
			}
			if err := sd.Decode(); err != nil {
				continue
			}
			all.Write(sd.Content)
		}
	}
	return all.String()
}

func base64Encode(s string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var sb strings.Builder
	b := []byte(s)
	for i := 0; i < len(b); i += 3 {
		var n uint32
		rem := len(b) - i
		n = uint32(b[i]) << 16
		if rem > 1 {
			n |= uint32(b[i+1]) << 8
		}
		if rem > 2 {
			n |= uint32(b[i+2])
		}
		sb.WriteByte(alphabet[(n>>18)&0x3F])
		sb.WriteByte(alphabet[(n>>12)&0x3F])
		if rem > 1 {
			sb.WriteByte(alphabet[(n>>6)&0x3F])
		} else {
			sb.WriteByte('=')
		}
		if rem > 2 {
			sb.WriteByte(alphabet[n&0x3F])
		} else {
			sb.WriteByte('=')
		}
	}
	return sb.String()
}

func runPdftotext(t *testing.T, pdfBytes []byte) string {
	t.Helper()
	cmd := exec.Command("pdftotext", "-layer", "-", "-")
	cmd.Stdin = bytes.NewReader(pdfBytes)
	out, err := cmd.Output()
	if err != nil {
		// Older poppler builds don't know -layer; fall back to plain text,
		// which (depending on poppler version) may or may not include
		// hidden-OCG text — either result is informative, so don't fail here.
		cmd = exec.Command("pdftotext", "-", "-")
		cmd.Stdin = bytes.NewReader(pdfBytes)
		out, err = cmd.Output()
		if err != nil {
			t.Fatalf("pdftotext: %v", err)
		}
	}
	return string(out)
}
