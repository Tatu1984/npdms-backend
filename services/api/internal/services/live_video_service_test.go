package services

import (
	"strings"
	"testing"
	"time"

	"github.com/npdms/api/internal/models"
)

func TestIngestTokens(t *testing.T) {
	tok := NewIngestToken()
	if !strings.HasPrefix(tok, "ing_") || len(tok) < 30 {
		t.Fatalf("token shape: %q", tok)
	}
	hash := HashIngestToken(tok)
	if len(hash) != 64 || strings.Contains(hash, tok) {
		t.Fatalf("hash shape: %q", hash)
	}
	if !VerifyIngestToken(tok, hash) {
		t.Error("own token refused")
	}
	rotated := NewIngestToken()
	if rotated == tok || VerifyIngestToken(tok, HashIngestToken(rotated)) {
		t.Error("old token accepted after rotation")
	}
	for _, bad := range []struct{ tok, hash string }{{"", hash}, {tok, ""}, {tok, "zz"}, {tok + "x", hash}} {
		if VerifyIngestToken(bad.tok, bad.hash) {
			t.Errorf("accepted %+v", bad)
		}
	}
	key := NewIngestKey()
	if len(key) != 16 || strings.ContainsAny(key, "/+=") {
		t.Errorf("key shape: %q", key)
	}
}

func TestClassifyPlaylist(t *testing.T) {
	now := time.Now()
	fresh, stale := now.Add(-3*time.Second), now.Add(-2*time.Minute)
	header := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n"
	withSegs := header + "#EXTINF:2.000000,\nindex1757926000.ts\n"
	cases := []struct {
		name string
		body []byte
		mod  time.Time
		want models.LiveStatus
	}{
		{"missing", nil, time.Time{}, models.LiveOffline},
		{"no segments", []byte(header), fresh, models.LiveConnecting},
		{"segments", []byte(withSegs), fresh, models.LiveOnline},
		{"ended", []byte(withSegs + "#EXT-X-ENDLIST\n"), fresh, models.LiveStopped},
		{"agent went quiet (omit_endlist)", []byte(withSegs), stale, models.LiveStopped},
		{"stale without segments", []byte(header), stale, models.LiveOffline},
	}
	for _, c := range cases {
		if got := ClassifyPlaylist(c.body, c.mod, now, 30*time.Second); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestPlaybackURL(t *testing.T) {
	if got := PlaybackURL("https://api.example.in/", "k123"); got != "https://api.example.in/api/edge/ingest/k123/index.m3u8" {
		t.Error(got)
	}
	cfg := edgeAgentConfig("https://api.example.in", "k123", "ing_x")
	if cfg.IngestURL != "https://api.example.in" || cfg.CameraID != "k123" || cfg.IngestToken != "ing_x" ||
		cfg.PublishURL != "https://api.example.in/api/edge/ingest/k123/index.m3u8" {
		t.Errorf("%+v", cfg)
	}
}
