package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The reordering is the part that is ours rather than the engine's, so it is
// tested on its own with no engine installed.
func TestReorderPreBaseVowels(t *testing.T) {
	cases := []struct {
		name   string
		script string
		read   string
		want   string
	}{
		{
			// The engine reads the glyphs left to right, so the i-kar lands one
			// consonant late: পুলিশ (police) comes back as পুলশি.
			name:   "Bengali i-kar",
			script: "Bengali",
			read:   "পুলশি",
			want:   "পুলিশ",
		},
		{
			name:   "Bengali e-kar",
			script: "Bengali",
			read:   "গ্রহণরে",
			want:   "গ্রহণের",
		},
		{
			name:   "Devanagari i-kar",
			script: "Devanagari",
			read:   "शकिायत",
			want:   "शिकायत",
		},
		{
			// Correct text must survive the pass untouched.
			name:   "already in order",
			script: "Bengali",
			read:   "কলকাতা পুলিশ",
			want:   "কলকাতা পুলিশ",
		},
		{
			// A script whose vowels render above the consonant is left alone.
			name:   "Kannada is not reordered",
			script: "Kannada",
			read:   "ದೂರುದಾರರು",
			want:   "ದೂರುದಾರರು",
		},
		{
			name:   "an unknown script is left alone",
			script: "",
			read:   "পুলশি",
			want:   "পুলশি",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reorderPreBaseVowels(c.read, c.script); got != c.want {
				t.Errorf("reordering %q as %s gave %q, want %q", c.read, c.script, got, c.want)
			}
		})
	}
}

// languagesFor must never ask the engine for a model this server does not
// have: tesseract fails the whole page rather than skipping the missing one.
func TestLanguagesForKeepsOnlyInstalledModels(t *testing.T) {
	e := &Extractor{languages: []string{"eng", "ben"}}

	got := e.languagesFor("Bengali")
	if strings.Join(got, "+") != "ben+eng" {
		t.Errorf("Bengali with only eng and ben installed gave %v, want [ben eng]", got)
	}

	// Tamil is not installed here, so the page is read as English rather than
	// failing outright.
	if got := e.languagesFor("Tamil"); strings.Join(got, "+") != "eng" {
		t.Errorf("Tamil with no Tamil model gave %v, want [eng]", got)
	}

	// An unknown script falls back to the default set, filtered the same way.
	if got := e.languagesFor("Klingon"); strings.Join(got, "+") != "eng+ben" {
		t.Errorf("an unknown script gave %v, want [eng ben]", got)
	}
}

// A deployment with no engine says so and extracts nothing, rather than
// returning empty text that reads like "this page is blank". This is the state
// of the Vercel API, which has no tesseract binary.
func TestWithoutAnEngineTheDocumentSaysSo(t *testing.T) {
	e := &Extractor{}

	result := e.scanned(context.Background(), "unused.png", "image/png", "This is an image")
	if result.Status != StatusOCRUnavailable {
		t.Errorf("status was %q, want %q", result.Status, StatusOCRUnavailable)
	}
	if result.Text != "" {
		t.Errorf("text was %q, want empty", result.Text)
	}
	if !strings.Contains(result.Note, "not available") {
		t.Errorf("the note does not say OCR is unavailable: %q", result.Note)
	}
}

// The end-to-end read, against whatever engine and models this machine has.
// It skips where tesseract is absent, so the suite still runs on a build box.
func TestReadsAScannedPage(t *testing.T) {
	e := NewExtractor()
	if !e.OCRAvailable() {
		t.Skip("tesseract is not installed — skipping the end-to-end OCR read")
	}

	page := filepath.Join("testdata", "scanned-english.png")
	if _, err := os.Stat(page); err != nil {
		t.Skipf("no sample page at %s", page)
	}

	result := e.Extract(context.Background(), page, "image/png", "scanned-english.png")
	if result.Status != StatusOCR {
		t.Fatalf("status was %q (%s), want %q", result.Status, result.Note, StatusOCR)
	}
	if result.OCR == nil {
		t.Fatal("no OCR detail was recorded")
	}

	// The words on the page must come back. This is English, where the engine
	// is strong; the Indic scripts are measured separately and are weaker.
	for _, word := range []string{"complainant", "stolen", "Gariahat"} {
		if !strings.Contains(strings.ToLower(result.Text), strings.ToLower(word)) {
			t.Errorf("the page does not contain %q; read back: %q", word, result.Text)
		}
	}

	if result.OCR.Engine == "" {
		t.Error("the engine that read the page was not recorded")
	}
	if len(result.OCR.Languages) == 0 {
		t.Error("the languages the page was read in were not recorded")
	}
	if result.OCR.Confidence <= 0 || result.OCR.Confidence > 1 {
		t.Errorf("confidence was %v, want a fraction above zero", result.OCR.Confidence)
	}
	// The note must tell an officer not to quote it.
	if !strings.Contains(result.Note, "not to quote it") {
		t.Errorf("the note does not warn that the text is unchecked: %q", result.Note)
	}
}
