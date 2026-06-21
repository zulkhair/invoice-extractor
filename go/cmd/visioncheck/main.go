// Command visioncheck is the vision sanity check (critical).
//
// It asks each candidate model to read the invoice number off a real image, and
// off a different control image. A model that reads the real one correctly AND
// returns a different answer for the control is genuinely reading pixels; identical
// answers mean a broken/ignored vision projector -> unusable.
//
//	go run ./cmd/visioncheck
//	go run ./cmd/visioncheck -image testdata/fixtures/x.png -expect INV-123
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"regexp"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"invoice-extractor/internal/config"
	"invoice-extractor/internal/ollama"
)

const controlToken = "CTRL-7788-XYZ"

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

func alnum(s string) string { return strings.ToUpper(nonAlnum.ReplaceAllString(s, "")) }

func main() {
	cfg := config.Load()
	image_ := flag.String("image", cfg.FixturesDir+"/synthetic_invoice_scan.png", "invoice image to read")
	expect := flag.String("expect", "INV/2026/06/0042", "expected invoice number on the image")
	modelsFlag := flag.String("models", strings.Join(cfg.BenchModels, ","), "comma-separated model tags")
	flag.Parse()

	mainImg, err := os.ReadFile(*image_)
	if err != nil {
		fmt.Fprintf(os.Stderr, "image not found: %s\n(generate: python ../python/scripts/make_synthetic_fixture.py)\n", *image_)
		os.Exit(2)
	}
	ctrlImg := controlImage()

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

	want := alnum(*expect)
	fmt.Printf("image:  %s\nexpect: %s\n\n", *image_, *expect)
	fmt.Printf("%-34s %-7s %-10s %s\n", "model", "reads?", "distinct?", "verdict")
	fmt.Println(strings.Repeat("-", 72))

	anyUsable := false
	for _, model := range splitNonEmpty(*modelsFlag) {
		aMain, err1 := ask(ctx, cl, model, mainImg, cfg.Temperature)
		aCtrl, err2 := ask(ctx, cl, model, ctrlImg, cfg.Temperature)
		if err1 != nil || err2 != nil {
			fmt.Printf("%-34s ERROR: %v\n", model, firstErr(err1, err2))
			continue
		}
		reads := strings.Contains(alnum(aMain), want)
		distinct := alnum(aMain) != alnum(aCtrl)
		usable := reads && distinct
		anyUsable = anyUsable || usable
		verdict := "FAIL"
		switch {
		case usable:
			verdict = "USABLE"
		case reads && !distinct:
			verdict = "BROKEN-PROJECTOR"
		}
		fmt.Printf("%-34s %-7v %-10v %s   main=%q\n", model, reads, distinct, verdict, aMain)
	}

	fmt.Println()
	if anyUsable {
		fmt.Println("PASS: at least one model demonstrably reads pixels. Record it as the vision model.")
		return
	}
	fmt.Println("FAIL: no model reliably read the image. Do not proceed with the vision path.")
	os.Exit(1)
}

func ask(ctx context.Context, cl *ollama.Client, model string, img []byte, temp float64) (string, error) {
	res, err := cl.Chat(ctx, model,
		"You read text off invoice images. Answer with only what is asked.",
		"What is the invoice number printed on this image? Reply with ONLY the invoice number.",
		[][]byte{img}, false, temp)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Content), nil
}

// controlImage renders a distinct invoice (number = controlToken) using basicfont,
// upscaled 3x so the glyphs are legible to a VLM.
func controlImage() []byte {
	small := image.NewRGBA(image.Rect(0, 0, 320, 170))
	draw.Draw(small, small.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	put := func(x, y int, s string) {
		d := &font.Drawer{Dst: small, Src: image.NewUniform(color.Black), Face: basicfont.Face7x13, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	put(12, 40, "CONTROL INVOICE")
	put(12, 80, "Invoice No: "+controlToken)
	put(12, 120, "Total: 1.234.567")

	big := image.NewRGBA(image.Rect(0, 0, 960, 510))
	draw.NearestNeighbor.Scale(big, big.Bounds(), small, small.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	_ = png.Encode(&buf, big)
	return buf.Bytes()
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

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
