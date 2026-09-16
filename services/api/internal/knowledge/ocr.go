package knowledge

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OCR reads text off a scanned page, in the languages of India.
//
// What it is for, stated plainly: OCR makes a scanned circular FINDABLE, not
// readable. Measured on clean rendered text, Tesseract returns most characters
// correctly in Latin and Devanagari, and rather fewer in Bengali, Tamil,
// Kannada and Odia — on a Bengali page, about half the characters and under
// half the words. That is poor for quoting and adequate for searching, because
// the document text is indexed with trigrams: against OCR output that scored
// 38% of words exactly, all seven Bengali terms tried (কলকাতা, পুলিশ, অভিযোগ,
// থানা, গ্রহণের, নথিভুক্ত, স্বীকার) still matched at 0.55 or better.
//
// So the text is stored with the engine, the languages, the script and the
// engine's own confidence, always labelled as unchecked, and the officer is
// shown the page rather than the transcription.

// DefaultLanguages is what a page is read as when its script cannot be
// detected. Every one of these is free, Apache-2.0, and installed with
// tesseract's language data.
var DefaultLanguages = []string{"eng", "ben", "hin"}

// scriptLanguages maps what Tesseract's own script detection reports to the
// language models to read the page with. English is included nearly everywhere
// because Indian official documents mix English words into every script.
var scriptLanguages = map[string][]string{
	"Latin":      {"eng"},
	"Bengali":    {"ben", "asm", "eng"},
	"Devanagari": {"hin", "mar", "nep", "san", "eng"},
	"Tamil":      {"tam", "eng"},
	"Telugu":     {"tel", "eng"},
	"Kannada":    {"kan", "eng"},
	"Malayalam":  {"mal", "eng"},
	"Gujarati":   {"guj", "eng"},
	"Gurmukhi":   {"pan", "eng"},
	"Oriya":      {"ori", "eng"},
	"Arabic":     {"urd", "eng"},
	"Han":        {"eng"},
}

// preBaseVowels are the dependent vowel signs that are WRITTEN to the left of
// the consonant they follow in the encoding. An engine reading glyphs from left
// to right emits them one consonant too late, so "পুলিশ" comes back as "পুলশি".
// Putting them back in order is arithmetic, not guesswork, and on a Bengali
// page it lifted characters from 38.5% to 49.7% and words from 29% to 38%.
var preBaseVowels = map[string]string{
	"Bengali":    "িেৈ", // ি ে ৈ
	"Devanagari": "ि",   // ि
	"Gujarati":   "િ",   // િ
	"Oriya":      "େୈ",  // େ ୈ
	"Tamil":      "ெேை", // ெ ே ை
	"Malayalam":  "െേൈ", // െ േ ൈ
	"Kannada":    "",    // renders above, not before
	"Telugu":     "",    // renders above, not before
	"Gurmukhi":   "ਿ",   // ਿ
}

// scriptConsonants is the code point range holding the consonants of each
// script, used only by the reordering above.
var scriptConsonants = map[string][2]rune{
	"Bengali":    {0x0995, 0x09b9},
	"Devanagari": {0x0915, 0x0939},
	"Gujarati":   {0x0a95, 0x0ab9},
	"Oriya":      {0x0b15, 0x0b39},
	"Tamil":      {0x0b95, 0x0bb9},
	"Malayalam":  {0x0d15, 0x0d39},
	"Gurmukhi":   {0x0a15, 0x0a39},
}

// OCRResult is one page-set read by the engine.
type OCRResult struct {
	Text       string
	Engine     string
	Languages  []string
	Script     string
	Confidence float64 // mean per-word confidence the engine reported, 0-1
	Pages      int
	Note       string
}

// maxOCRPages bounds the work one upload can ask for. A circular is a few
// pages; a scanned manual is not something to read synchronously.
const maxOCRPages = 20

// ocrPageTimeout bounds one page.
const ocrPageTimeout = 45 * time.Second

// readScanned runs the OCR stage over an image or a scanned PDF.
func (e *Extractor) readScanned(ctx context.Context, path, contentType string) (OCRResult, error) {
	pages, cleanup, err := e.pageImages(ctx, path, contentType)
	if err != nil {
		return OCRResult{}, err
	}
	defer cleanup()

	if len(pages) == 0 {
		return OCRResult{}, fmt.Errorf("no page images to read")
	}

	// The script is detected once, from the first page: a circular does not
	// change script halfway through, and detection is the slowest part.
	script := e.detectScript(ctx, pages[0])
	languages := e.languagesFor(script)

	var texts []string
	var confidences []float64
	for i, page := range pages {
		text, confidence, err := e.readPage(ctx, page, languages)
		if err != nil {
			// A page that cannot be read does not lose the pages that could.
			continue
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		texts = append(texts, text)
		if confidence > 0 {
			confidences = append(confidences, confidence)
		}
		if i+1 >= maxOCRPages {
			break
		}
	}

	if len(texts) == 0 {
		return OCRResult{Engine: e.engineVersion(), Languages: languages, Script: script, Pages: len(pages)},
			fmt.Errorf("the engine read no text from %d page(s)", len(pages))
	}

	text := clean(strings.Join(texts, "\n\n"))
	if reordered := reorderPreBaseVowels(text, script); reordered != text {
		text = reordered
	}
	if len(text) > MaxTextBytes {
		text = text[:MaxTextBytes]
	}

	return OCRResult{
		Text:       text,
		Engine:     e.engineVersion(),
		Languages:  languages,
		Script:     script,
		Confidence: mean(confidences),
		Pages:      len(texts),
	}, nil
}

// pageImages turns the upload into page images. An image is itself one page; a
// PDF is rasterised with pdftoppm, which ships with pdftotext.
func (e *Extractor) pageImages(ctx context.Context, path, contentType string) ([]string, func(), error) {
	noop := func() {}

	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return []string{path}, noop, nil
	}

	if e.pdftoppm == "" {
		return nil, noop, fmt.Errorf("pdftoppm is not installed, so a scanned PDF cannot be rasterised")
	}

	dir, err := os.MkdirTemp("", "npdms-ocr-")
	if err != nil {
		return nil, noop, err
	}
	cleanup := func() { os.RemoveAll(dir) }

	ctx, cancel := context.WithTimeout(ctx, time.Duration(maxOCRPages)*ocrPageTimeout)
	defer cancel()

	// 300 dpi greyscale is what the engine is trained for; -l bounds the work.
	cmd := exec.CommandContext(ctx, e.pdftoppm, "-r", "300", "-gray", "-png",
		"-l", strconv.Itoa(maxOCRPages), path, filepath.Join(dir, "page"))
	if err := cmd.Run(); err != nil {
		cleanup()
		return nil, noop, fmt.Errorf("the PDF could not be rasterised: %w", err)
	}

	entries, err := filepath.Glob(filepath.Join(dir, "page*.png"))
	if err != nil || len(entries) == 0 {
		cleanup()
		return nil, noop, fmt.Errorf("the PDF produced no page images")
	}
	sort.Strings(entries)
	return entries, cleanup, nil
}

var scriptLine = regexp.MustCompile(`(?m)^Script:\s*(.+)$`)

// detectScript asks Tesseract which script the page is in. It returns "" when
// the page carries too little text to tell, which is common and not an error:
// the caller then reads it with the default language set.
func (e *Extractor) detectScript(ctx context.Context, page string) string {
	ctx, cancel := context.WithTimeout(ctx, ocrPageTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.tesseract, page, "stdout", "--psm", "0")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	if m := scriptLine.FindSubmatch(out); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}

// languagesFor chooses the language models for a script, keeping only those
// this server actually has. Asking tesseract for a model that is not installed
// fails the whole page, so a deployment with a partial language pack reads what
// it can rather than nothing.
func (e *Extractor) languagesFor(script string) []string {
	wanted, ok := scriptLanguages[script]
	if !ok {
		wanted = DefaultLanguages
	}

	installed := make(map[string]bool, len(e.languages))
	for _, lang := range e.languages {
		installed[lang] = true
	}

	kept := make([]string, 0, len(wanted))
	for _, lang := range wanted {
		if len(installed) == 0 || installed[lang] {
			kept = append(kept, lang)
		}
	}
	if len(kept) == 0 {
		// Nothing for this script is installed. English is always present.
		return []string{"eng"}
	}
	return kept
}

// readPage reads one page and returns its text with the engine's mean word
// confidence. The TSV output carries a confidence per word, which is the
// engine's opinion of its own reading — useful for telling an officer how much
// to trust it, and never a measure of whether the reading is right.
func (e *Extractor) readPage(ctx context.Context, page string, languages []string) (string, float64, error) {
	ctx, cancel := context.WithTimeout(ctx, ocrPageTimeout)
	defer cancel()

	// TSV carries a confidence per word, which is worth having. It depends on
	// a config file shipped with tesseract, and an installation that keeps its
	// language data somewhere non-standard may not find it — so a failure here
	// falls back to plain text rather than losing the page.
	out, err := exec.CommandContext(ctx, e.tesseract, page, "stdout",
		"-l", strings.Join(languages, "+"), "--psm", "6", "tsv").Output()
	if err != nil || !looksLikeTSV(out) {
		plain, plainErr := exec.CommandContext(ctx, e.tesseract, page, "stdout",
			"-l", strings.Join(languages, "+"), "--psm", "6").Output()
		if plainErr != nil {
			return "", 0, plainErr
		}
		return clean(string(plain)), 0, nil
	}

	var words []string
	var confidences []float64
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 0, 64*1024), MaxTextBytes)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 12 || fields[0] == "level" {
			continue
		}
		word := strings.TrimSpace(fields[11])
		if word == "" {
			continue
		}
		words = append(words, word)
		if c, err := strconv.ParseFloat(fields[10], 64); err == nil && c >= 0 {
			confidences = append(confidences, c/100)
		}
	}

	return strings.Join(words, " "), mean(confidences), nil
}

// reorderPreBaseVowels puts dependent vowel signs back where the encoding says
// they belong. See the note on preBaseVowels.
func reorderPreBaseVowels(text, script string) string {
	vowels, ok := preBaseVowels[script]
	if !ok || vowels == "" {
		return text
	}
	span, ok := scriptConsonants[script]
	if !ok {
		return text
	}

	isConsonant := func(r rune) bool { return r >= span[0] && r <= span[1] }
	isPreBase := func(r rune) bool { return strings.ContainsRune(vowels, r) }

	runes := []rune(text)
	for i := 2; i < len(runes); i++ {
		if isPreBase(runes[i]) && isConsonant(runes[i-1]) && isConsonant(runes[i-2]) {
			runes[i-1], runes[i] = runes[i], runes[i-1]
		}
	}
	return string(runes)
}

// looksLikeTSV reports whether tesseract produced its tab-separated output
// rather than plain text or an error page.
func looksLikeTSV(out []byte) bool {
	first := string(out)
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	return strings.Count(first, "\t") >= 10
}

func (e *Extractor) engineVersion() string {
	if e.tesseractVersion != "" {
		return e.tesseractVersion
	}
	return "tesseract"
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}
