// Package config loads service configuration from the environment (stdlib only).
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	OllamaHost    string
	OllamaTimeout time.Duration

	// Bench candidate pool (all Q4, all vision-capable).
	ModelPrimary  string
	ModelSmall    string
	ModelFallback string
	ModelText     string // text-only NuExtract 1.x — bench candidate only
	BenchModels   []string

	// Live pipeline models.
	TextPathModel   string
	VisionPathModel string

	TextLayerMinChars int
	RasterDPI         int
	RasterMaxPx       int
	Temperature       float64

	PDFBackend  string // "poppler" (default) | "gofitz"
	FixturesDir string
	LabelsDir   string
	Port        string
}

func Load() Config {
	c := Config{
		OllamaHost:        env("OLLAMA_HOST", "http://127.0.0.1:11434"),
		OllamaTimeout:     time.Duration(envInt("OLLAMA_TIMEOUT_S", 180)) * time.Second,
		ModelPrimary:      env("MODEL_PRIMARY", "frob/nuextract-2.0:8b-q4_K_M"),
		ModelSmall:        env("MODEL_SMALL", "qwen2.5vl:3b-q4_K_M"),
		ModelFallback:     env("MODEL_FALLBACK", "qwen2.5vl:7b-q4_K_M"),
		ModelText:         env("MODEL_TEXT", "nuextract"),
		TextPathModel:     env("TEXT_PATH_MODEL", env("MODEL_SMALL", "qwen2.5vl:3b-q4_K_M")),
		VisionPathModel:   env("VISION_PATH_MODEL", env("MODEL_FALLBACK", "qwen2.5vl:7b-q4_K_M")),
		TextLayerMinChars: envInt("TEXT_LAYER_MIN_CHARS", 100),
		RasterDPI:         envInt("RASTER_DPI", 200),
		RasterMaxPx:       envInt("RASTER_MAX_PX", 1600),
		Temperature:       envFloat("LLM_TEMPERATURE", 0),
		PDFBackend:        env("PDF_BACKEND", "poppler"),
		FixturesDir:       env("FIXTURES_DIR", "testdata/fixtures"),
		LabelsDir:         env("LABELS_DIR", "testdata/labels"),
		Port:              env("PORT", "8000"),
	}
	if bm := os.Getenv("BENCH_MODELS"); bm != "" {
		for _, m := range strings.Split(bm, ",") {
			if m = strings.TrimSpace(m); m != "" {
				c.BenchModels = append(c.BenchModels, m)
			}
		}
	} else {
		c.BenchModels = []string{c.ModelSmall, c.ModelPrimary, c.ModelFallback}
	}
	return c
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
