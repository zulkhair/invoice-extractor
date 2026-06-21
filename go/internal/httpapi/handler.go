// Package httpapi exposes /health and /extract.
package httpapi

import (
	"encoding/json"
	"net/http"

	"invoice-extractor/internal/config"
	"invoice-extractor/internal/ollama"
	"invoice-extractor/internal/pdftext"
	"invoice-extractor/internal/pipeline"
)

const maxUpload = 32 << 20 // 32 MiB

// NewMux wires the routes. The server starts and /health serves even when Ollama
// is down (the process is healthy; the response says whether extraction will work).
func NewMux(cl *ollama.Client, be pdftext.Backend, cfg config.Config) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(cl, cfg))
	mux.HandleFunc("POST /extract", extractHandler(cl, be, cfg))
	return mux
}

func healthHandler(cl *ollama.Client, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ollamaState := "down"
		if cl.Ping(r.Context()) {
			ollamaState = "up"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":            "ok",
			"ollama":            ollamaState,
			"ollama_host":       cfg.OllamaHost,
			"pdf_backend":       cfg.PDFBackend,
			"text_path_model":   cfg.TextPathModel,
			"vision_path_model": cfg.VisionPathModel,
		})
	}
}

func extractHandler(cl *ollama.Client, be pdftext.Backend, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(maxUpload); err != nil {
			writeError(w, http.StatusBadRequest, "could not parse multipart form: "+err.Error())
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing 'file' upload")
			return
		}
		defer file.Close()

		data := make([]byte, 0, header.Size)
		buf := make([]byte, 64<<10)
		for {
			n, readErr := file.Read(buf)
			data = append(data, buf[:n]...)
			if readErr != nil {
				break
			}
		}
		if len(data) == 0 {
			writeError(w, http.StatusBadRequest, "empty upload")
			return
		}

		res, err := pipeline.Extract(r.Context(), data, header.Filename, cl, be, cfg)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"invoice": res.Invoice,
			"metadata": map[string]any{
				"path":        res.Path,
				"model":       res.Model,
				"latency_s":   res.LatencySeconds,
				"fell_back":   res.FellBack,
				"consistent":  res.Invoice.Consistent,
				"warnings":    res.Warnings,
				"filename":    header.Filename,
			},
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
