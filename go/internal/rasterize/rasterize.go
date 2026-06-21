// Package rasterize renders PDF pages to PNG (poppler `pdftoppm`) and normalizes
// loaded images to clamped PNGs for the vision path.
package rasterize

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"

	"golang.org/x/image/draw"

	// Image format decoders for the image path.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// PDFToPNGs renders each page of a PDF to a normalized PNG (one per page).
func PDFToPNGs(pdf []byte, dpi, maxPx int) ([][]byte, error) {
	dir, err := os.MkdirTemp("", "raster-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		return nil, err
	}
	prefix := filepath.Join(dir, "page")
	// pdftoppm -png -r DPI in.pdf prefix  ->  prefix-1.png, prefix-2.png, ...
	cmd := exec.Command("pdftoppm", "-png", "-r", strconv.Itoa(dpi), in, prefix)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftoppm failed (is poppler-utils installed?): %v: %s", err, stderr.String())
	}

	matches, _ := filepath.Glob(prefix + "-*.png")
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("pdftoppm produced no pages")
	}
	out := make([][]byte, 0, len(matches))
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		norm, err := normalizePNG(raw, maxPx)
		if err != nil {
			return nil, err
		}
		out = append(out, norm)
	}
	return out, nil
}

// ImageToPNG decodes any supported image and normalizes it to a clamped PNG.
func ImageToPNG(data []byte, maxPx int) ([]byte, error) {
	return normalizePNG(data, maxPx)
}

func normalizePNG(data []byte, maxPx int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	longest := w
	if h > longest {
		longest = h
	}
	if maxPx > 0 && longest > maxPx {
		scale := float64(maxPx) / float64(longest)
		nw, nh := int(float64(w)*scale), int(float64(h)*scale)
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		img = dst
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// IsImage sniffs common invoice image formats by magic bytes.
func IsImage(d []byte) bool {
	switch {
	case len(d) >= 8 && bytes.Equal(d[:8], []byte("\x89PNG\r\n\x1a\n")):
		return true
	case len(d) >= 3 && bytes.Equal(d[:3], []byte{0xff, 0xd8, 0xff}): // JPEG
		return true
	case len(d) >= 4 && (bytes.Equal(d[:4], []byte("II*\x00")) || bytes.Equal(d[:4], []byte("MM\x00*"))): // TIFF
		return true
	case len(d) >= 4 && bytes.Equal(d[:4], []byte("RIFF")): // WEBP
		return true
	default:
		return false
	}
}
