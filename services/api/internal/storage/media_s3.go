package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// maxMediaObjectBytes bounds what a read will buffer. A two-second segment is a
// few megabytes at most; anything far larger is not a live segment.
const maxMediaObjectBytes = 64 << 20

// S3MediaStore keeps HLS files in an S3-compatible bucket — Cloudflare R2 in
// production (durable, effectively unlimited, no egress fees), or S3 or MinIO.
// Objects are keyed <prefix>/<streamKey>/<file>.
//
// Every call runs under a short deadline (R2_TIMEOUT_MS, default 4 s; uploads
// get twice that). Without it, wrong credentials or an unreachable endpoint
// make the S3 client retry with backoff for a long time, which hangs a
// serverless function until the platform kills it and the caller sees no
// response at all. The deadline also caps the client's retries: a bad
// credential fails fast and visibly.
type S3MediaStore struct {
	client     *minio.Client
	bucket     string
	prefix     string
	publicBase string
	timeout    time.Duration
	http       *http.Client
	backend    string
}

func NewS3MediaStore(cfg MediaConfig) (*S3MediaStore, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%w: endpoint %q is not a URL such as https://<account>.r2.cloudflarestorage.com", ErrMediaNotConfigured, cfg.Endpoint)
	}
	secure := u.Scheme != "http"
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 4 * time.Second
	}

	transport, err := minio.DefaultTransport(secure)
	if err != nil {
		return nil, err
	}
	transport.DialContext = (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = timeout
	transport.ResponseHeaderTimeout = 2 * timeout

	client, err := minio.New(u.Host, &minio.Options{
		Creds:     credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:    secure,
		Region:    cfg.Region, // "auto" is the R2 idiom
		Transport: transport,
		// Path-style addressing: required by R2, MinIO and most S3-compatible
		// providers.
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMediaNotConfigured, err)
	}
	return &S3MediaStore{
		client:     client,
		bucket:     cfg.Bucket,
		prefix:     cfg.Prefix,
		publicBase: cfg.PublicBase,
		timeout:    timeout,
		http:       &http.Client{Timeout: timeout},
		backend:    cfg.Backend,
	}, nil
}

func (s *S3MediaStore) Backend() string { return s.backend }

func (s *S3MediaStore) objectKey(streamKey, file string) (string, error) {
	if SafeMediaKey(streamKey) == "" {
		return "", errors.New("unsafe stream key")
	}
	if file != "" && SafeMediaName(file) == "" {
		return "", errors.New("unsafe file name")
	}
	return s.prefix + "/" + streamKey + "/" + file, nil
}

func (s *S3MediaStore) Put(ctx context.Context, streamKey, file string, body []byte) error {
	key, err := s.objectKey(streamKey, file)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*s.timeout)
	defer cancel()
	ct, kind := MediaContentType(file)
	_, err = s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{
		ContentType:  ct,
		CacheControl: MediaCacheControl(kind),
		// A known size and a single part: no multipart for a few megabytes.
		DisableMultipart: true,
	})
	return err
}

func (s *S3MediaStore) Get(ctx context.Context, streamKey, file string) (*MediaObject, error) {
	key, err := s.objectKey(streamKey, file)
	if err != nil {
		return nil, err
	}
	ct, _ := MediaContentType(file)
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// With a public/CDN base configured, read through it (cheaper at scale). The
	// bytes still come back through the API, so playback stays gated: handing
	// the browser the public URL would bypass the purpose-logged session.
	if s.publicBase != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.publicBase+"/"+key, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Cache-Control", "no-store")
		resp, err := s.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
			return nil, ErrNotFound
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("public media base answered %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxMediaObjectBytes))
		if err != nil {
			return nil, err
		}
		mod, _ := http.ParseTime(resp.Header.Get("Last-Modified"))
		return &MediaObject{Body: body, ContentType: ct, ModTime: mod}, nil
	}

	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer obj.Close()
	info, err := obj.Stat()
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(obj, maxMediaObjectBytes))
	if err != nil {
		return nil, err
	}
	return &MediaObject{Body: body, ContentType: ct, ModTime: info.LastModified}, nil
}

func (s *S3MediaStore) Delete(ctx context.Context, streamKey, file string) error {
	key, err := s.objectKey(streamKey, file)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	err = s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil && isNotFound(err) {
		return nil
	}
	return err
}

// Purge deletes the playlist first, then every segment under the key. Bounded
// by a few deadlines; whatever it cannot reach is left for a bucket lifecycle
// rule to collect.
func (s *S3MediaStore) Purge(ctx context.Context, streamKey string) error {
	if err := s.Delete(ctx, streamKey, "index.m3u8"); err != nil {
		return err
	}
	prefix, err := s.objectKey(streamKey, "")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 4*s.timeout)
	defer cancel()
	objects := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true})
	toDelete := make(chan minio.ObjectInfo)
	go func() {
		defer close(toDelete)
		for o := range objects {
			if o.Err != nil {
				return
			}
			if !strings.HasPrefix(o.Key, prefix) {
				continue
			}
			select {
			case toDelete <- o:
			case <-ctx.Done():
				return
			}
		}
	}()
	var firstErr error
	for e := range s.client.RemoveObjects(ctx, s.bucket, toDelete, minio.RemoveObjectsOptions{}) {
		if e.Err != nil && firstErr == nil {
			firstErr = e.Err
		}
	}
	return firstErr
}
