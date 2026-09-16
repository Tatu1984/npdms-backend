// Package knowledge reads searchable text out of uploaded documents.
//
// Extraction is honest about what it could do. A PDF with a text layer is
// read with pdftotext. A scanned PDF or an image has no text to read; OCR is
// an optional stage that needs tesseract on the server, and when it is absent
// the document is recorded as findable by its metadata only — text is never
// invented or guessed.
package knowledge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Status values match the extraction_status check constraint.
const (
	StatusTextLayer      = "TEXT_LAYER"
	StatusPlainText      = "PLAIN_TEXT"
	StatusOCR            = "OCR"
	StatusNoTextLayer    = "NO_TEXT_LAYER"
	StatusOCRUnavailable = "OCR_UNAVAILABLE"
	StatusUnsupported    = "UNSUPPORTED"
	StatusFailed         = "FAILED"
)

// MaxTextBytes bounds the text stored per document.
const MaxTextBytes = 2 << 20

// minMeaningfulRunes is how much readable text a PDF must yield before it is
// treated as having a text layer rather than being a scan with stray marks.
const minMeaningfulRunes = 20

type Result struct {
	Text   string
	Status string
	Note   string

	// Set when the text was read off an image rather than taken from the file.
	OCR *OCRResult
}

// Extractor runs the available tools. Paths are resolved once at start-up so
// every document states the same capability.
type Extractor struct {
	pdftotext        string
	pdftoppm         string
	tesseract        string
	tesseractVersion string
	languages        []string
}

func NewExtractor() *Extractor {
	e := &Extractor{}
	e.pdftotext, _ = exec.LookPath("pdftotext")
	e.pdftoppm, _ = exec.LookPath("pdftoppm")
	e.tesseract, _ = exec.LookPath("tesseract")
	if e.tesseract != "" {
		e.tesseractVersion = readTesseractVersion(e.tesseract)
		e.languages = readInstalledLanguages(e.tesseract)
	}
	return e
}

// Languages reports the language models installed here, so a screen can say
// which of India's languages this deployment can actually read.
func (e *Extractor) Languages() []string { return e.languages }

// EngineVersion reports the OCR engine, or "" where there is none.
func (e *Extractor) EngineVersion() string { return e.tesseractVersion }

func readTesseractVersion(bin string) string {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return "tesseract"
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if line == "" {
		return "tesseract"
	}
	return line
}

func readInstalledLanguages(bin string) []string {
	out, err := exec.Command(bin, "--list-langs").Output()
	if err != nil {
		return nil
	}
	var langs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// The first line is a header; osd is script detection, not a language.
		if line == "" || strings.HasSuffix(line, ":") || line == "osd" {
			continue
		}
		langs = append(langs, line)
	}
	return langs
}

// OCRAvailable reports whether the optional OCR stage can run here.
func (e *Extractor) OCRAvailable() bool { return e.tesseract != "" }

// Extract reads text from the file at path. contentType and filename decide
// the method; the file itself is never modified.
func (e *Extractor) Extract(ctx context.Context, path, contentType, filename string) Result {
	ext := strings.ToLower(filepath.Ext(filename))
	ct := strings.ToLower(contentType)

	switch {
	case ct == "application/pdf" || ext == ".pdf":
		return e.pdf(ctx, path)
	case strings.HasPrefix(ct, "text/plain") || ext == ".txt":
		return plainText(path)
	case strings.HasPrefix(ct, "image/"):
		return e.scanned(ctx, path, ct, "This is an image")
	default:
		return Result{Status: StatusUnsupported,
			Note: "Text is not extracted from this file type. The document is findable by its title, reference and description."}
	}
}

func (e *Extractor) pdf(ctx context.Context, path string) Result {
	if e.pdftotext == "" {
		return Result{Status: StatusUnsupported,
			Note: "pdftotext is not installed on this server, so PDF text was not read. The document is findable by its metadata."}
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, e.pdftotext, "-enc", "UTF-8", "-q", path, "-")
	cmd.Stdout = &limitWriter{w: &out, n: MaxTextBytes}
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		note := "The PDF could not be read"
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			note += ": " + truncate(msg, 200)
		}
		return Result{Status: StatusFailed, Note: note + ". The document is findable by its metadata."}
	}
	text := clean(out.String())
	if meaningfulRunes(text) < minMeaningfulRunes {
		return e.scanned(ctx, path, "application/pdf", "This PDF has no text layer — it appears to be scanned")
	}
	return Result{Text: text, Status: StatusTextLayer, Note: "Text read from the PDF's text layer."}
}

// scanned reads a page that carries no machine-readable text.
func (e *Extractor) scanned(ctx context.Context, path, contentType, what string) Result {
	if e.tesseract == "" {
		return Result{Status: StatusOCRUnavailable,
			Note: what + ". OCR is not available on this server (tesseract is not installed), so no text was extracted. The document is findable by its metadata only."}
	}

	result, err := e.readScanned(ctx, path, contentType)
	if err != nil {
		return Result{Status: StatusNoTextLayer,
			Note: what + ". OCR could not read it: " + truncate(err.Error(), 200) +
				". The document is findable by its metadata only."}
	}

	note := fmt.Sprintf("%s. Read by %s from %d page(s) as %s.",
		what, result.Engine, result.Pages, strings.Join(result.Languages, "+"))
	if result.Script != "" {
		note += " Script detected: " + result.Script + "."
	}
	if result.Confidence > 0 {
		note += fmt.Sprintf(" The engine reports %.0f%% mean confidence in its own reading.", result.Confidence*100)
	}
	note += " This text was read by machine and has not been checked: use it to find the document, not to quote it."

	return Result{Text: result.Text, Status: StatusOCR, Note: note, OCR: &result}
}

func plainText(path string) Result {
	f, err := os.Open(path)
	if err != nil {
		return Result{Status: StatusFailed, Note: "The text file could not be read."}
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxTextBytes))
	if err != nil {
		return Result{Status: StatusFailed, Note: "The text file could not be read."}
	}
	if !utf8.Valid(b) {
		return Result{Status: StatusFailed, Note: "The text file is not UTF-8; save it as UTF-8 and upload again."}
	}
	return Result{Text: clean(string(b)), Status: StatusPlainText, Note: "Text read from a plain-text file."}
}

// clean removes NUL and form-feed characters and collapses runs of blank lines.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\f", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.TrimRightFunc(l, unicode.IsSpace)
		if l == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func meaningfulRunes(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// limitWriter stops accepting bytes after n, silently, so an enormous PDF
// cannot exhaust memory. Truncation is recorded by the caller's length cap.
type limitWriter struct {
	w io.Writer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	if len(p) > l.n {
		_, err := l.w.Write(p[:l.n])
		l.n = 0
		return len(p), err
	}
	l.n -= len(p)
	return l.w.Write(p)
}
