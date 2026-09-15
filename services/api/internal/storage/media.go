package storage

// Live video media store — where the Edge Agent's HLS uploads land.
//
// Ported from the Live Feed Portal (src/lib/media/store.ts) and its KMCP port.
// The Edge Agent PUTs a playlist (index.m3u8) and two-second segments (.ts) for
// each camera; those bytes must land somewhere durable and be served back to
// officers. The interface is deliberately tiny — put, get, delete, purge — and
// is separate from the evidence Store: live video is a rolling window that is
// rewritten every two seconds, not a record with a custody hash.
//
// Backends, chosen by MEDIA_BACKEND independently of STORAGE_BACKEND, so
// evidence can stay where it is:
//
//   - r2 / s3 / minio → any S3-compatible store (Cloudflare R2 in production).
//     Works on Vercel, because the bytes live in the bucket, not on Vercel's
//     ephemeral filesystem.
//   - fs → a directory on this server. For development or a single disk-backed
//     edge server. Refused on Vercel.
//   - database is refused outright: a camera writes a segment every two seconds,
//     around 30 000 objects a day, which would bloat Postgres.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MediaObject is one stored HLS file.
type MediaObject struct {
	Body        []byte
	ContentType string
	// ModTime is when the object was last written. Liveness uses it to tell a
	// feed that is still arriving from one whose agent has gone quiet.
	ModTime time.Time
}

// MediaStore holds HLS playlists and segments keyed by camera stream key.
type MediaStore interface {
	// Put stores bytes for <streamKey>/<file>, replacing any existing object.
	Put(ctx context.Context, streamKey, file string, body []byte) error
	// Get reads one object; ErrNotFound when it does not exist.
	Get(ctx context.Context, streamKey, file string) (*MediaObject, error)
	// Delete removes one object (the agent DELETEs segments that left the
	// window). Deleting something absent is not an error.
	Delete(ctx context.Context, streamKey, file string) error
	// Purge removes everything stored for a stream key, playlist first. Best
	// effort: used when streaming is disabled or a camera decommissioned.
	Purge(ctx context.Context, streamKey string) error
	// Backend names the implementation for diagnostics.
	Backend() string
}

/* --------------------------------------------------------- shared helpers -- */

// ContentKind drives the cache policy.
type ContentKind int

const (
	KindOther ContentKind = iota
	KindPlaylist
	KindSegment
)

var (
	segmentExt  = map[string]bool{".ts": true, ".m4s": true, ".mp4": true, ".aac": true, ".vtt": true}
	playlistExt = map[string]bool{".m3u8": true}
	safeNameRe  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	safeKeyRe   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// SafeMediaName allows only a flat HLS file name — .m3u8, .ts, .m4s, .mp4, .aac
// or .vtt — and blocks traversal and junk uploads. It returns "" when the name
// is not acceptable.
//
// Stricter than the reference, which took the last path segment of whatever it
// was given: a name carrying any separator is refused here rather than
// silently shortened, because the Edge Agent only ever writes flat names next
// to its playlist.
func SafeMediaName(file string) string {
	name := strings.TrimPrefix(file, "/")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return ""
	}
	if !safeNameRe.MatchString(name) {
		return ""
	}
	ext := strings.ToLower(path.Ext(name))
	if !segmentExt[ext] && !playlistExt[ext] {
		return ""
	}
	return name
}

// SafeMediaKey allows a stream key that is a single safe path segment.
func SafeMediaKey(key string) string {
	k := strings.TrimSpace(key)
	if k == "" || k == "." || k == ".." || len(k) > 64 || !safeKeyRe.MatchString(k) {
		return ""
	}
	return k
}

// MediaContentType returns the MIME type and cache kind for an HLS file name.
func MediaContentType(file string) (string, ContentKind) {
	f := strings.ToLower(file)
	switch {
	case strings.HasSuffix(f, ".m3u8"):
		return "application/vnd.apple.mpegurl", KindPlaylist
	case strings.HasSuffix(f, ".ts"):
		return "video/mp2t", KindSegment
	case strings.HasSuffix(f, ".m4s"), strings.HasSuffix(f, ".mp4"):
		return "video/mp4", KindSegment
	case strings.HasSuffix(f, ".aac"):
		return "audio/aac", KindSegment
	case strings.HasSuffix(f, ".vtt"):
		return "text/vtt", KindOther
	}
	return "application/octet-stream", KindOther
}

// MediaCacheControl is the cache policy, critical for a working live feed:
// playlists change every ~2 s and must never be cached; a segment never changes
// once written, so it is immutable and can be cached hard.
func MediaCacheControl(kind ContentKind) string {
	switch kind {
	case KindPlaylist:
		return "no-cache, no-store, must-revalidate"
	case KindSegment:
		return "private, max-age=31536000, immutable"
	}
	return "private, max-age=60"
}

/* -------------------------------------------------------------- selection -- */

// ErrMediaNotConfigured wraps every reason the live video store cannot be used.
var ErrMediaNotConfigured = errors.New("live video storage is not configured")

// MediaConfig describes the live video store.
type MediaConfig struct {
	Backend string // r2 | s3 | minio | fs

	FSDir string

	Endpoint   string // https://<account>.r2.cloudflarestorage.com
	Region     string // "auto" for R2
	AccessKey  string
	SecretKey  string
	Bucket     string
	Prefix     string // default "hls"
	PublicBase string // optional public/CDN base for reads

	Timeout time.Duration

	// OnVercel is true when running as a Vercel function.
	OnVercel bool
	// missing lists required settings that were not found, by env name.
	missing []string
	// refusal is set when the selection itself is refused.
	refusal string
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

const r2Hint = "set MEDIA_BACKEND=r2 with R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY and R2_BUCKET"

// MediaConfigFromEnv reads the live video store configuration.
//
// Variable names follow the Live Feed Portal's .env.example (MEDIA_BACKEND,
// R2_*), with KMCP's S3_* names and MinIO's accepted as alternatives.
//
// When MEDIA_BACKEND is unset it follows STORAGE_BACKEND: filesystem → fs,
// minio → the same MinIO server; database is refused, never inherited.
func MediaConfigFromEnv() MediaConfig {
	cfg := MediaConfig{OnVercel: os.Getenv("VERCEL") != ""}

	backend := strings.ToLower(strings.TrimSpace(os.Getenv("MEDIA_BACKEND")))
	inherited := false
	if backend == "" {
		inherited = true
		switch strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_BACKEND"))) {
		case "minio":
			backend = "minio"
		case "database":
			backend = "database"
		default:
			backend = "fs"
		}
	}
	if backend == "filesystem" {
		backend = "fs"
	}
	cfg.Backend = backend

	ms := 4000
	if v, err := strconv.Atoi(firstEnv("R2_TIMEOUT_MS", "MEDIA_TIMEOUT_MS")); err == nil && v > 0 {
		ms = v
	}
	cfg.Timeout = time.Duration(ms) * time.Millisecond

	switch backend {
	case "database":
		how := "MEDIA_BACKEND=database"
		if inherited {
			how = "STORAGE_BACKEND=database (MEDIA_BACKEND is unset, so live video would follow it)"
		}
		cfg.refusal = fmt.Sprintf("%s cannot hold live video: a camera writes a segment every two seconds, "+
			"which would bloat Postgres. Configure Cloudflare R2 for live video — %s. Evidence can stay on the database backend.", how, r2Hint)

	case "fs":
		cfg.FSDir = firstEnv("MEDIA_FS_DIR")
		if cfg.FSDir == "" {
			root := firstEnv("STORAGE_PATH")
			if root == "" {
				root = "./data"
			}
			cfg.FSDir = strings.TrimRight(root, "/") + "/hls"
		}
		if cfg.OnVercel {
			how := fmt.Sprintf("MEDIA_BACKEND is %q", strings.TrimSpace(os.Getenv("MEDIA_BACKEND")))
			if inherited {
				how = "MEDIA_BACKEND is unset, so live video would use the filesystem"
			}
			cfg.refusal = fmt.Sprintf("%s — the filesystem backend cannot be used on Vercel "+
				"(its filesystem is ephemeral and read-only); %s", how, r2Hint)
		}

	case "r2", "s3", "minio":
		cfg.Prefix = strings.Trim(firstEnv("R2_PREFIX", "HLS_KEY_PREFIX", "MEDIA_PREFIX"), "/")
		if cfg.Prefix == "" {
			cfg.Prefix = "hls"
		}
		cfg.PublicBase = strings.TrimRight(firstEnv("R2_PUBLIC_BASE", "HLS_PUBLIC_BASE"), "/")
		cfg.Region = firstEnv("R2_REGION", "S3_REGION")
		if cfg.Region == "" {
			cfg.Region = "auto"
		}
		if backend == "minio" {
			cfg.Endpoint = firstEnv("S3_ENDPOINT", "MINIO_ENDPOINT")
			if cfg.Endpoint != "" && !strings.Contains(cfg.Endpoint, "://") {
				scheme := "http://"
				if ok, _ := strconv.ParseBool(os.Getenv("MINIO_USE_SSL")); ok {
					scheme = "https://"
				}
				cfg.Endpoint = scheme + cfg.Endpoint
			}
			cfg.AccessKey = firstEnv("S3_ACCESS_KEY_ID", "MINIO_ACCESS_KEY")
			cfg.SecretKey = firstEnv("S3_SECRET_ACCESS_KEY", "MINIO_SECRET_KEY")
			cfg.Bucket = firstEnv("S3_BUCKET", "MINIO_BUCKET")
			if cfg.Region == "auto" {
				cfg.Region = "us-east-1"
			}
		} else {
			cfg.Endpoint = firstEnv("R2_ENDPOINT", "S3_ENDPOINT")
			if cfg.Endpoint == "" {
				if acct := firstEnv("R2_ACCOUNT_ID"); acct != "" {
					cfg.Endpoint = "https://" + acct + ".r2.cloudflarestorage.com"
				}
			}
			cfg.AccessKey = firstEnv("R2_ACCESS_KEY_ID", "S3_ACCESS_KEY_ID")
			cfg.SecretKey = firstEnv("R2_SECRET_ACCESS_KEY", "S3_SECRET_ACCESS_KEY")
			cfg.Bucket = firstEnv("R2_BUCKET", "S3_BUCKET")
		}
		if cfg.Endpoint == "" {
			cfg.missing = append(cfg.missing, "R2_ACCOUNT_ID (or S3_ENDPOINT)")
		}
		if cfg.AccessKey == "" {
			cfg.missing = append(cfg.missing, "R2_ACCESS_KEY_ID")
		}
		if cfg.SecretKey == "" {
			cfg.missing = append(cfg.missing, "R2_SECRET_ACCESS_KEY")
		}
		if cfg.Bucket == "" {
			cfg.missing = append(cfg.missing, "R2_BUCKET")
		}

	default:
		cfg.refusal = fmt.Sprintf("unknown MEDIA_BACKEND %q (use r2, s3, minio or fs)", backend)
	}
	return cfg
}

// OpenMedia builds the configured live video store. Opening never touches the
// network, so a misconfigured bucket costs nothing at start-up; the first
// upload or read fails fast instead (short timeouts), with the reason logged.
//
// Every error wraps ErrMediaNotConfigured and says what to set. The API keeps
// running without a store: ingest answers 503 with the reason and every camera
// reads OFFLINE.
func OpenMedia(cfg MediaConfig) (MediaStore, error) {
	if cfg.refusal != "" {
		return nil, fmt.Errorf("%w: %s", ErrMediaNotConfigured, cfg.refusal)
	}
	// Each constructor's result is checked before it becomes an interface value:
	// a nil *T inside a MediaStore would not compare equal to nil.
	switch cfg.Backend {
	case "fs":
		st, err := NewFilesystemMediaStore(cfg.FSDir)
		if err != nil {
			return nil, err
		}
		return st, nil
	case "r2", "s3", "minio":
		if len(cfg.missing) > 0 {
			return nil, fmt.Errorf("%w: MEDIA_BACKEND=%s needs %s", ErrMediaNotConfigured, cfg.Backend, strings.Join(cfg.missing, ", "))
		}
		st, err := NewS3MediaStore(cfg)
		if err != nil {
			return nil, err
		}
		return st, nil
	}
	return nil, fmt.Errorf("%w: unknown MEDIA_BACKEND %q", ErrMediaNotConfigured, cfg.Backend)
}
