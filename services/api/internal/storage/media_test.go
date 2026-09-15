package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeMediaName(t *testing.T) {
	ok := []string{"index.m3u8", "/index.m3u8", "index1757926000.ts", "seg-1.m4s", "init.mp4", "a.aac", "subs.vtt", "INDEX.M3U8"}
	for _, n := range ok {
		if SafeMediaName(n) == "" {
			t.Errorf("%q should be accepted", n)
		}
	}
	bad := []string{"", "/", "..", "../index.m3u8", "a/index.m3u8", `a\index.m3u8`, "index..m3u8", "evil.sh", "index.m3u8.exe",
		"index m3u8.ts", "%2e%2e.ts", "x.html", "noext"}
	for _, n := range bad {
		if SafeMediaName(n) != "" {
			t.Errorf("%q should be refused", n)
		}
	}
}

func TestSafeMediaKey(t *testing.T) {
	if SafeMediaKey("AbC_12-xyZ") == "" {
		t.Error("base64url key refused")
	}
	for _, k := range []string{"", ".", "..", "a/b", "a b", strings.Repeat("a", 65), "a%2F"} {
		if SafeMediaKey(k) != "" {
			t.Errorf("%q should be refused", k)
		}
	}
}

func TestCachePolicy(t *testing.T) {
	ct, kind := MediaContentType("index.m3u8")
	if ct != "application/vnd.apple.mpegurl" || kind != KindPlaylist || !strings.Contains(MediaCacheControl(kind), "no-store") {
		t.Errorf("playlist: %s %v %s", ct, kind, MediaCacheControl(kind))
	}
	ct, kind = MediaContentType("index1.ts")
	if ct != "video/mp2t" || kind != KindSegment || !strings.Contains(MediaCacheControl(kind), "immutable") {
		t.Errorf("segment: %s %v %s", ct, kind, MediaCacheControl(kind))
	}
}

func withEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{"MEDIA_BACKEND", "STORAGE_BACKEND", "VERCEL", "MEDIA_FS_DIR", "STORAGE_PATH",
		"R2_ACCOUNT_ID", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY", "R2_BUCKET", "R2_ENDPOINT",
		"S3_ENDPOINT", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY", "S3_BUCKET", "MINIO_ENDPOINT"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestMediaSelectionRefusals(t *testing.T) {
	dir := t.TempDir()

	withEnv(t, map[string]string{"MEDIA_BACKEND": "fs", "VERCEL": "1", "MEDIA_FS_DIR": dir})
	if _, err := OpenMedia(MediaConfigFromEnv()); !errors.Is(err, ErrMediaNotConfigured) || !strings.Contains(err.Error(), "Vercel") || !strings.Contains(err.Error(), "R2_BUCKET") {
		t.Errorf("fs on Vercel: %v", err)
	}

	withEnv(t, map[string]string{"VERCEL": "1"}) // unset MEDIA_BACKEND defaults to fs → refused too
	if _, err := OpenMedia(MediaConfigFromEnv()); err == nil || !strings.Contains(err.Error(), "Vercel") {
		t.Errorf("unset backend on Vercel: %v", err)
	}

	withEnv(t, map[string]string{"MEDIA_BACKEND": "database"})
	if _, err := OpenMedia(MediaConfigFromEnv()); err == nil || !strings.Contains(err.Error(), "bloat Postgres") {
		t.Errorf("database backend: %v", err)
	}

	withEnv(t, map[string]string{"STORAGE_BACKEND": "database"}) // inherited, as on the Vercel staging API
	if _, err := OpenMedia(MediaConfigFromEnv()); err == nil || !strings.Contains(err.Error(), "STORAGE_BACKEND=database") {
		t.Errorf("inherited database backend: %v", err)
	}

	withEnv(t, map[string]string{"MEDIA_BACKEND": "r2 ", "R2_ACCOUNT_ID": "acct"})
	if _, err := OpenMedia(MediaConfigFromEnv()); err == nil || !strings.Contains(err.Error(), "R2_ACCESS_KEY_ID") || !strings.Contains(err.Error(), "R2_BUCKET") {
		t.Errorf("r2 missing credentials: %v", err)
	}

	withEnv(t, map[string]string{"MEDIA_BACKEND": "r2", "R2_ACCOUNT_ID": "acct", "R2_ACCESS_KEY_ID": "k", "R2_SECRET_ACCESS_KEY": "s", "R2_BUCKET": "b"})
	cfg := MediaConfigFromEnv()
	st, err := OpenMedia(cfg)
	if err != nil || st.Backend() != "r2" || cfg.Endpoint != "https://acct.r2.cloudflarestorage.com" || cfg.Region != "auto" || cfg.Prefix != "hls" {
		t.Errorf("r2 config: %v %+v", err, cfg)
	}
}

func TestFilesystemMediaStore(t *testing.T) {
	dir := t.TempDir()
	st, err := NewFilesystemMediaStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := st.Get(ctx, "cam1key", "index.m3u8"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing object: %v", err)
	}
	if err := st.Put(ctx, "cam1key", "index.m3u8", []byte("#EXTM3U")); err != nil {
		t.Fatal(err)
	}
	obj, err := st.Get(ctx, "cam1key", "index.m3u8")
	if err != nil || string(obj.Body) != "#EXTM3U" || obj.ModTime.IsZero() {
		t.Errorf("get: %v %+v", err, obj)
	}
	if err := st.Put(ctx, "../escape", "index.m3u8", []byte("x")); err == nil {
		t.Error("traversal key accepted")
	}
	if err := st.Put(ctx, "cam1key", "../../x.ts", []byte("x")); err == nil {
		t.Error("traversal name accepted")
	}
	if err := st.Delete(ctx, "cam1key", "absent.ts"); err != nil {
		t.Errorf("delete absent: %v", err)
	}
	if err := st.Purge(ctx, "cam1key"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cam1key")); !os.IsNotExist(err) {
		t.Errorf("purge left the directory: %v", err)
	}
}
