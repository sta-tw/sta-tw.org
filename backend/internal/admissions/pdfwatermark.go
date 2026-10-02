package admissions

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func init() {
	// This process never wants pdfcpu reading/writing ~/.config — it only
	// ever touches brochure PDFs in memory, and a missing/unwritable config
	// dir on the container would otherwise make the first call fail.
	api.DisableConfigDir()
}

// sourceMarkerPlain is the fact we need to recover from a leaked copy: that
// it came from this platform. It's deliberately plain English/ASCII so it
// survives base64 round-tripping without escaping concerns in a PDF literal
// string (base64's alphabet never needs \( \) \\ escaping).
const sourceMarkerPlain = "SOURCE=STA-ADMISSIONS-PLATFORM"

// applyHiddenWatermark embeds an obfuscated, hidden-by-default Optional
// Content layer into the brochure, scattered as several fragments across
// pages rather than one block, so it survives naive "shrink this PDF" tools
// (which recompress streams but don't analyze/strip OCG layers) and doesn't
// show up as one obvious chunk in a casual `pdftotext` dump. This only needs
// to run once, at publish time — the result is what gets served publicly.
func applyHiddenWatermark(pdfBytes []byte) ([]byte, error) {
	conf := model.NewStatelessConfiguration()

	ctx, err := api.ReadValidateAndOptimize(context.Background(), bytes.NewReader(pdfBytes), conf, nil)
	if err != nil {
		return nil, fmt.Errorf("watermark: read pdf: %w", err)
	}
	if err := ctx.EnsurePageCount(); err != nil {
		return nil, fmt.Errorf("watermark: page count: %w", err)
	}
	if ctx.PageCount < 1 {
		return nil, fmt.Errorf("watermark: pdf has no pages")
	}

	ocgRef, err := ensureHiddenOCProperties(ctx)
	if err != nil {
		return nil, fmt.Errorf("watermark: optional content setup: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(sourceMarkerPlain))
	fragments := splitIntoFragments(encoded, fragmentCount(ctx.PageCount))
	pages := fragmentPageNumbers(ctx.PageCount, len(fragments))
	for i, pageNr := range pages {
		if err := stampHiddenFragment(ctx, pageNr, *ocgRef, fragments[i]); err != nil {
			return nil, fmt.Errorf("watermark: page %d: %w", pageNr, err)
		}
	}

	var out bytes.Buffer
	if err := api.Write(context.Background(), ctx, &out, conf); err != nil {
		return nil, fmt.Errorf("watermark: write pdf: %w", err)
	}
	return out.Bytes(), nil
}

// ensureHiddenOCProperties makes sure the document's catalog has an Optional
// Content layer whose default state is OFF (hidden on open, not included
// when a viewer's "select all" copies visible text). It merges into any
// OCProperties the source document already has instead of clobbering it —
// some scanned brochures legitimately carry their own OCR/layer structure.
// The layer's /Name is deliberately innocuous: a name like "Watermark"
// invites exactly the curiosity this is meant to avoid.
func ensureHiddenOCProperties(ctx *model.Context) (*types.IndirectRef, error) {
	rootDict, err := ctx.Catalog()
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}

	ocgDict := types.Dict{
		"Name": types.StringLiteral("Layer1"),
		"Type": types.Name("OCG"),
		"Usage": types.Dict{
			"View":   types.Dict{"ViewState": types.Name("OFF")},
			"Print":  types.Dict{"PrintState": types.Name("OFF")},
			"Export": types.Dict{"ExportState": types.Name("OFF")},
		},
	}
	ocgRef, err := ctx.IndRefForNewObject(ocgDict)
	if err != nil {
		return nil, fmt.Errorf("create optional content group: %w", err)
	}

	existing, hasExisting := rootDict.Find("OCProperties")
	if !hasExisting {
		rootDict.Update("OCProperties", types.Dict{
			"OCGs": types.Array{*ocgRef},
			"D": types.Dict{
				"OFF":      types.Array{*ocgRef},
				"ON":       types.Array{},
				"Order":    types.Array{},
				"RBGroups": types.Array{},
			},
		})
		return ocgRef, nil
	}

	ocProps, err := ctx.DereferenceDict(existing)
	if err != nil || ocProps == nil {
		return nil, fmt.Errorf("existing optional content properties: %w", err)
	}
	ocgs, _ := ctx.DereferenceArray(ocProps["OCGs"])
	ocProps.Update("OCGs", append(ocgs, *ocgRef))

	configObj, hasConfig := ocProps.Find("D")
	config, err := ctx.DereferenceDict(configObj)
	if err != nil || config == nil || !hasConfig {
		config = types.Dict{"ON": types.Array{}, "Order": types.Array{}, "RBGroups": types.Array{}}
	}
	off, _ := ctx.DereferenceArray(config["OFF"])
	config.Update("OFF", append(off, *ocgRef))
	ocProps.Update("D", config)

	return ocgRef, nil
}

// stampHiddenFragment adds one invisible text fragment to a single page,
// wrapped in a /OC marked-content section referencing ocgRef. It only adds
// to the page's existing Resources/Contents — original content is untouched.
func stampHiddenFragment(ctx *model.Context, pageNr int, ocgRef types.IndirectRef, fragment string) error {
	pageDict, _, inhPAttrs, err := ctx.PageDict(context.Background(), pageNr, true)
	if err != nil {
		return fmt.Errorf("page dict: %w", err)
	}
	if pageDict == nil {
		return fmt.Errorf("page %d not found", pageNr)
	}
	resources := inhPAttrs.Resources
	if resources == nil {
		resources = types.Dict{}
	}

	properties := dereferenceOrNewDict(ctx, resources, "Properties")
	properties.Update("MC0", ocgRef)
	resources.Update("Properties", properties)

	fonts := dereferenceOrNewDict(ctx, resources, "Font")
	if _, exists := fonts.Find("WMFont"); !exists {
		fontRef, err := ctx.IndRefForNewObject(types.Dict{
			"Type":     types.Name("Font"),
			"Subtype":  types.Name("Type1"),
			"BaseFont": types.Name("Helvetica"),
			"Encoding": types.Name("WinAnsiEncoding"),
		})
		if err != nil {
			return fmt.Errorf("font resource: %w", err)
		}
		fonts.Update("WMFont", *fontRef)
	}
	resources.Update("Font", fonts)

	pageDict.Update("Resources", resources)

	streamRef, err := buildHiddenContentStream(ctx, fragment)
	if err != nil {
		return fmt.Errorf("content stream: %w", err)
	}
	return appendPageContentStream(ctx, pageDict, *streamRef)
}

func dereferenceOrNewDict(ctx *model.Context, parent types.Dict, key string) types.Dict {
	obj, ok := parent.Find(key)
	if !ok {
		return types.Dict{}
	}
	d, err := ctx.DereferenceDict(obj)
	if err != nil || d == nil {
		return types.Dict{}
	}
	return d
}

// buildHiddenContentStream renders fragment as text positioned just off the
// visible page area (belt-and-suspenders alongside the OCG state itself) and
// wraps it in BDC/EMC tagged to /MC0, the Properties entry pointing at the
// hidden OCG.
func buildHiddenContentStream(ctx *model.Context, fragment string) (*types.IndirectRef, error) {
	var buf bytes.Buffer
	buf.WriteString("q\n/OC /MC0 BDC\nBT\n/WMFont 1 Tf\n1 0 0 1 1 1 Tm\n(")
	buf.WriteString(fragment)
	buf.WriteString(") Tj\nET\nEMC\nQ\n")

	sd := types.StreamDict{
		Dict:           types.Dict{},
		Content:        buf.Bytes(),
		FilterPipeline: []types.PDFFilter{{Name: filter.Flate}},
	}
	sd.InsertName("Filter", filter.Flate)
	if err := sd.Encode(); err != nil {
		return nil, err
	}
	return ctx.IndRefForNewObject(sd)
}

// appendPageContentStream adds newStreamRef to a page's /Contents without
// disturbing existing content — PDF content streams concatenate, so adding
// one more is always additive regardless of whether /Contents was a single
// stream ref or already an array.
func appendPageContentStream(ctx *model.Context, pageDict types.Dict, newStreamRef types.IndirectRef) error {
	obj, found := pageDict.Find("Contents")
	if !found {
		pageDict.Insert("Contents", types.Array{newStreamRef})
		return nil
	}
	resolved, err := ctx.Dereference(obj)
	if err != nil {
		return fmt.Errorf("dereference contents: %w", err)
	}
	if arr, ok := resolved.(types.Array); ok {
		pageDict.Update("Contents", append(arr, newStreamRef))
		return nil
	}
	if ir, ok := obj.(types.IndirectRef); ok {
		pageDict.Update("Contents", types.Array{ir, newStreamRef})
		return nil
	}
	pageDict.Update("Contents", types.Array{obj, newStreamRef})
	return nil
}

// fragmentCount keeps the marker in up to 3 pieces (fewer if the brochure is
// shorter) so no single page carries the whole, recognizable string.
func fragmentCount(pageCount int) int {
	if pageCount >= 3 {
		return 3
	}
	return pageCount
}

// fragmentPageNumbers spreads fragments across the document (roughly at the
// 1/4, 1/2, 3/4 marks for 3 fragments) rather than the first/last page,
// which is the first place a casual stripping attempt looks.
func fragmentPageNumbers(pageCount, n int) []int {
	if pageCount <= 0 || n <= 0 {
		return nil
	}
	pages := make([]int, n)
	for i := 0; i < n; i++ {
		pos := (i + 1) * pageCount / (n + 1)
		if pos < 1 {
			pos = 1
		}
		if pos > pageCount {
			pos = pageCount
		}
		pages[i] = pos
	}
	return pages
}

func splitIntoFragments(s string, n int) []string {
	if n <= 1 || len(s) <= n {
		return []string{s}
	}
	chunk := (len(s) + n - 1) / n
	fragments := make([]string, 0, n)
	for i := 0; i < len(s); i += chunk {
		end := i + chunk
		if end > len(s) {
			end = len(s)
		}
		fragments = append(fragments, s[i:end])
	}
	return fragments
}
