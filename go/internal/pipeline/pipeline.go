// Package pipeline routes a document through the text-layer path or the vision
// path, with fallback.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"invoice-extractor/internal/config"
	"invoice-extractor/internal/ollama"
	"invoice-extractor/internal/pdftext"
	"invoice-extractor/internal/postprocess"
	"invoice-extractor/internal/prompts"
	"invoice-extractor/internal/rasterize"
	"invoice-extractor/internal/schema"
)

type Result struct {
	Invoice        schema.Invoice
	Path           string // "text" | "vision"
	Model          string
	LatencySeconds float64
	FellBack       bool
	Warnings       []string
}

var jsonObjRe = regexp.MustCompile(`(?s)\{.*\}`)

// ParseModelContent parses raw model output into a canonical Invoice (wire ->
// normalize). Returns ok=false if no JSON object can be recovered.
func ParseModelContent(content string) (schema.Invoice, bool) {
	raw, ok := unmarshalLenient(content)
	if !ok {
		return schema.Invoice{}, false
	}
	return postprocess.Normalize(raw), true
}

func unmarshalLenient(content string) (schema.RawInvoice, bool) {
	var raw schema.RawInvoice
	if json.Unmarshal([]byte(content), &raw) == nil {
		return raw, true
	}
	if m := jsonObjRe.FindString(content); m != "" {
		if json.Unmarshal([]byte(m), &raw) == nil {
			return raw, true
		}
	}
	return schema.RawInvoice{}, false
}

// Extract runs the hybrid pipeline over one document.
func Extract(
	ctx context.Context,
	data []byte,
	filename string,
	cl *ollama.Client,
	be pdftext.Backend,
	cfg config.Config,
) (Result, error) {
	var warnings []string

	switch {
	case pdftext.IsPDF(data):
		layer, err := be.Detect(data)
		if err != nil {
			return Result{}, fmt.Errorf("text-layer detection: %w", err)
		}
		if layer.HasTextLayer {
			if inv, chat, ok := tryText(ctx, layer.Text, cl, cfg); ok && inv.HasRequiredish() {
				return finish(inv, "text", chat, warnings, false), nil
			}
			warnings = append(warnings, "text path weak/failed -> falling back to vision")
			return visionPDF(ctx, data, cl, cfg, warnings, true)
		}
		return visionPDF(ctx, data, cl, cfg, warnings, false)

	case rasterize.IsImage(data):
		png, err := rasterize.ImageToPNG(data, cfg.RasterMaxPx)
		if err != nil {
			return Result{}, err
		}
		return visionImages(ctx, [][]byte{png}, cl, cfg, warnings, false)

	default:
		return Result{}, fmt.Errorf("unsupported file type (not PDF or known image): %s", filename)
	}
}

func tryText(ctx context.Context, text string, cl *ollama.Client, cfg config.Config) (schema.Invoice, ollama.ChatResult, bool) {
	chat, err := cl.Chat(ctx, cfg.TextPathModel, prompts.System(), prompts.TextUser(text), nil, true, cfg.Temperature)
	if err != nil {
		return schema.Invoice{}, ollama.ChatResult{}, false
	}
	inv, ok := ParseModelContent(chat.Content)
	if !ok {
		return schema.Invoice{}, chat, false
	}
	return inv, chat, true
}

func visionPDF(ctx context.Context, data []byte, cl *ollama.Client, cfg config.Config, warnings []string, fellBack bool) (Result, error) {
	pngs, err := rasterize.PDFToPNGs(data, cfg.RasterDPI, cfg.RasterMaxPx)
	if err != nil {
		return Result{}, err
	}
	return visionImages(ctx, pngs, cl, cfg, warnings, fellBack)
}

func visionImages(ctx context.Context, images [][]byte, cl *ollama.Client, cfg config.Config, warnings []string, fellBack bool) (Result, error) {
	chat, err := cl.Chat(ctx, cfg.VisionPathModel, prompts.System(), prompts.VisionUser(), images, true, cfg.Temperature)
	if err != nil {
		return Result{}, fmt.Errorf("vision path: %w", err)
	}
	inv, ok := ParseModelContent(chat.Content)
	if !ok {
		return Result{}, fmt.Errorf("vision model did not return schema-valid JSON")
	}
	return finish(inv, "vision", chat, warnings, fellBack), nil
}

func finish(inv schema.Invoice, path string, chat ollama.ChatResult, warnings []string, fellBack bool) Result {
	if warnings == nil {
		warnings = []string{}
	}
	return Result{
		Invoice:        inv,
		Path:           path,
		Model:          chat.Model,
		LatencySeconds: chat.Latency.Seconds(),
		FellBack:       fellBack,
		Warnings:       warnings,
	}
}
