// Command server runs the invoice-extractor HTTP API.
package main

import (
	"log"
	"net/http"

	"invoice-extractor/internal/config"
	"invoice-extractor/internal/httpapi"
	"invoice-extractor/internal/ollama"
	"invoice-extractor/internal/pdftext"
)

func main() {
	cfg := config.Load()

	cl, err := ollama.New(cfg.OllamaHost, cfg.OllamaTimeout)
	if err != nil {
		log.Fatalf("ollama client: %v", err)
	}
	be := pdftext.Poppler{MinChars: cfg.TextLayerMinChars}

	mux := httpapi.NewMux(cl, be, cfg)
	addr := ":" + cfg.Port
	log.Printf("invoice-extractor listening on %s (ollama=%s, pdf=%s)", addr, cfg.OllamaHost, cfg.PDFBackend)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
