// Command bench runs every candidate model over the labeled fixtures and prints
// a field-level accuracy scorecard + median latency (the decision tool).
//
//	go run ./cmd/bench
//	go run ./cmd/bench -models qwen2.5vl:7b-q4_K_M
//
// Fixtures: testdata/fixtures/<stem>.{pdf,png,jpg,...} with ground truth in
// testdata/labels/<stem>.json. All candidates are compared on the VISION path.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"invoice-extractor/internal/config"
	"invoice-extractor/internal/ollama"
	"invoice-extractor/internal/pipeline"
	"invoice-extractor/internal/postprocess"
	"invoice-extractor/internal/prompts"
	"invoice-extractor/internal/rasterize"
	"invoice-extractor/internal/schema"
	"invoice-extractor/internal/scoring"
)

var imgExt = map[string]bool{
	".pdf": true, ".png": true, ".jpg": true, ".jpeg": true,
	".tif": true, ".tiff": true, ".webp": true,
}

type fixture struct{ stem, path, label string }

type modelSummary struct {
	model                       string
	n, fails                    int
	acc, liPrec, liRec, medLat  float64
	fieldHits, fieldSeen        map[string]int
}

func main() {
	cfg := config.Load()
	modelsFlag := flag.String("models", strings.Join(cfg.BenchModels, ","), "comma-separated model tags")
	flag.Parse()
	models := splitNonEmpty(*modelsFlag)

	fixtures := discover(cfg)
	if len(fixtures) == 0 {
		fmt.Fprintf(os.Stderr, "No labeled fixtures in %s (+ %s).\n"+
			"Generate synthetic ones: python ../python/scripts/make_synthetic_fixture.py\n",
			cfg.FixturesDir, cfg.LabelsDir)
		os.Exit(2)
	}

	cl, err := ollama.New(cfg.OllamaHost, cfg.OllamaTimeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx := context.Background()
	if !cl.Ping(ctx) {
		fmt.Fprintf(os.Stderr, "Ollama not reachable at %s — start it first.\n", cfg.OllamaHost)
		os.Exit(2)
	}

	fmt.Printf("fixtures: %d   models: %d   host: %s\n\n", len(fixtures), len(models), cfg.OllamaHost)
	sysPrompt := prompts.System()

	var summaries []modelSummary
	for _, model := range models {
		s := modelSummary{model: model, fieldHits: map[string]int{}, fieldSeen: map[string]int{}}
		var accs, precs, recs, lats []float64
		for _, fx := range fixtures {
			gold, err := loadGold(fx.label)
			if err != nil {
				fmt.Printf("  [%s] %s: bad label: %v\n", model, fx.stem, err)
				s.fails++
				continue
			}
			images, err := toImages(fx.path, cfg)
			if err != nil {
				fmt.Printf("  [%s] %s: %v\n", model, fx.stem, err)
				s.fails++
				continue
			}
			chat, err := cl.Chat(ctx, model, sysPrompt, prompts.VisionUser(), images, true, cfg.Temperature)
			if err != nil {
				fmt.Printf("  [%s] %s: %v\n", model, fx.stem, err)
				s.fails++
				continue
			}
			lats = append(lats, chat.Latency.Seconds())
			pred, ok := pipeline.ParseModelContent(chat.Content)
			if !ok {
				fmt.Printf("  [%s] %s: parse failure\n", model, fx.stem)
				s.fails++
				continue
			}
			sc := scoring.Score(pred, gold)
			accs = append(accs, sc.Accuracy)
			precs = append(precs, sc.LineItemPrecision)
			recs = append(recs, sc.LineItemRecall)
			for _, f := range scoring.ScalarFields {
				s.fieldSeen[f]++
				if sc.PerField[f] {
					s.fieldHits[f]++
				}
			}
			fmt.Printf("  [%s] %s: acc=%.2f li_p=%.2f li_r=%.2f %.1fs\n",
				model, fx.stem, sc.Accuracy, sc.LineItemPrecision, sc.LineItemRecall, chat.Latency.Seconds())
		}
		s.n = len(accs)
		s.acc = mean(accs)
		s.liPrec = mean(precs)
		s.liRec = mean(recs)
		s.medLat = median(lats)
		summaries = append(summaries, s)
	}

	printScorecard(summaries)
}

func discover(cfg config.Config) []fixture {
	entries, _ := filepath.Glob(filepath.Join(cfg.FixturesDir, "*"))
	sort.Strings(entries)
	var out []fixture
	for _, p := range entries {
		if !imgExt[strings.ToLower(filepath.Ext(p))] {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		label := filepath.Join(cfg.LabelsDir, stem+".json")
		if _, err := os.Stat(label); err == nil {
			out = append(out, fixture{stem: stem, path: p, label: label})
		}
	}
	return out
}

func loadGold(label string) (schema.Invoice, error) {
	data, err := os.ReadFile(label)
	if err != nil {
		return schema.Invoice{}, err
	}
	var raw schema.RawInvoice
	if err := json.Unmarshal(data, &raw); err != nil {
		return schema.Invoice{}, err
	}
	return postprocess.Normalize(raw), nil
}

func toImages(path string, cfg config.Config) ([][]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(path), ".pdf") {
		return rasterize.PDFToPNGs(data, cfg.RasterDPI, cfg.RasterMaxPx)
	}
	png, err := rasterize.ImageToPNG(data, cfg.RasterMaxPx)
	if err != nil {
		return nil, err
	}
	return [][]byte{png}, nil
}

func printScorecard(summaries []modelSummary) {
	fmt.Println("\n=== SCORECARD (field accuracy = mean over fixtures) ===")
	fmt.Printf("%-34s %3s %4s %9s %8s %7s %7s\n", "model", "n", "fail", "field_acc", "li_prec", "li_rec", "med_s")
	fmt.Println(strings.Repeat("-", 80))
	for _, s := range summaries {
		fmt.Printf("%-34s %3d %4d %9.3f %8.3f %7.3f %7.1f\n",
			s.model, s.n, s.fails, s.acc, s.liPrec, s.liRec, s.medLat)
	}

	fmt.Println("\n=== PER-FIELD ACCURACY ===")
	fmt.Printf("%-18s", "field")
	for _, s := range summaries {
		fmt.Printf("%14s", truncate(strings.SplitN(s.model, ":", 2)[0], 12))
	}
	fmt.Println()
	for _, f := range scoring.ScalarFields {
		fmt.Printf("%-18s", f)
		for _, s := range summaries {
			seen := s.fieldSeen[f]
			v := 0.0
			if seen > 0 {
				v = float64(s.fieldHits[f]) / float64(seen)
			}
			fmt.Printf("%14.2f", v)
		}
		fmt.Println()
	}
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
