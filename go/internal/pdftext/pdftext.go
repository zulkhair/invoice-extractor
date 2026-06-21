// Package pdftext detects and extracts an embedded PDF text layer.
//
// Backend decision: poppler shell-out (pdftotext), NOT go-fitz. Rationale: go-fitz
// needs CGo + MuPDF dev headers at build time; poppler keeps the build pure-Go and
// trivially cross-compilable, at the cost of a runtime dependency on the poppler
// binaries. The Backend interface lets go-fitz be swapped in later.
package pdftext

import (
	"bytes"
	"os/exec"
	"strings"
)

// Backend abstracts text-layer extraction so the engine can be swapped.
type Backend interface {
	Detect(pdf []byte) (TextLayer, error)
}

type TextLayer struct {
	Text         string
	HasTextLayer bool
	PageCount    int
	CharsPerPage []int
}

// Poppler shells out to `pdftotext`. MinChars is the per-page threshold for
// deciding a page carries a usable text layer.
type Poppler struct {
	MinChars int
}

func (p Poppler) Detect(pdf []byte) (TextLayer, error) {
	// `pdftotext -layout - -` reads the PDF from stdin and writes text to stdout,
	// with a form-feed (\f) between pages.
	cmd := exec.Command("pdftotext", "-layout", "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return TextLayer{}, wrap(err, stderr.String())
	}

	text := out.String()
	pages := strings.Split(text, "\f")
	// pdftotext appends a trailing \f; drop the empty tail page.
	if n := len(pages); n > 1 && strings.TrimSpace(pages[n-1]) == "" {
		pages = pages[:n-1]
	}
	counts := make([]int, len(pages))
	maxChars := 0
	for i, pg := range pages {
		counts[i] = len(strings.TrimSpace(pg))
		if counts[i] > maxChars {
			maxChars = counts[i]
		}
	}
	return TextLayer{
		Text:         strings.TrimSpace(text),
		HasTextLayer: maxChars >= p.MinChars,
		PageCount:    len(pages),
		CharsPerPage: counts,
	}, nil
}

func wrap(err error, stderr string) error {
	if s := strings.TrimSpace(stderr); s != "" {
		return &execError{msg: "pdftotext: " + s, err: err}
	}
	return &execError{msg: "pdftotext failed (is poppler-utils installed?)", err: err}
}

type execError struct {
	msg string
	err error
}

func (e *execError) Error() string { return e.msg }
func (e *execError) Unwrap() error  { return e.err }

// IsPDF reports whether data looks like a PDF.
func IsPDF(data []byte) bool {
	return len(data) >= 5 && bytes.Equal(data[:5], []byte("%PDF-"))
}
