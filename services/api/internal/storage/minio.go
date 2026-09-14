package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioStore keeps objects in S3-compatible storage.
type MinioStore struct {
	client *minio.Client
	bucket string
}

// NewMinioStore connects and ensures the bucket exists. It fails rather than
// degrading: a caller that asked for MinIO should be told it is unavailable,
// not silently given something else.
func NewMinioStore(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinioStore, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}

	return &MinioStore{client: client, bucket: bucket}, nil
}

func (s *MinioStore) Backend() string { return "minio" }

func (s *MinioStore) Put(ctx context.Context, key string, r io.Reader, contentType string) (*Object, error) {
	// Hash as the bytes stream past on their way to the bucket, so the digest
	// describes exactly what was stored without a second read.
	digest := sha256.New()
	tee := io.TeeReader(r, digest)

	info, err := s.client.PutObject(ctx, s.bucket, key, tee, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, err
	}

	return &Object{
		Key:         key,
		Size:        info.Size,
		ContentType: contentType,
		SHA256:      hex.EncodeToString(digest.Sum(nil)),
		StoredAt:    time.Now(),
	}, nil
}

func (s *MinioStore) Get(ctx context.Context, key string) (io.ReadCloser, *Object, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, err
	}
	stat, err := object.Stat()
	if err != nil {
		object.Close()
		if isNotFound(err) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	return object, &Object{
		Key:         key,
		Size:        stat.Size,
		ContentType: stat.ContentType,
		StoredAt:    stat.LastModified,
	}, nil
}

func (s *MinioStore) Stat(ctx context.Context, key string) (*Object, error) {
	stat, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &Object{
		Key:         key,
		Size:        stat.Size,
		ContentType: stat.ContentType,
		StoredAt:    stat.LastModified,
	}, nil
}

func (s *MinioStore) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *MinioStore) Hash(ctx context.Context, key string) (string, int64, error) {
	body, _, err := s.Get(ctx, key)
	if err != nil {
		return "", 0, err
	}
	defer body.Close()

	digest := sha256.New()
	size, err := io.Copy(digest, body)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	response := minio.ToErrorResponse(err)
	return response.Code == "NoSuchKey" || response.StatusCode == 404 ||
		strings.Contains(err.Error(), "does not exist")
}

/* --------------------------------- selection ------------------------------ */

// Config describes which backend to use.
type Config struct {
	Backend        string // "filesystem" (default) or "minio"
	FilesystemRoot string
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioUseSSL    bool
}

// FromEnv reads storage configuration.
//
// Filesystem is the default. A single edge server has local disk, and evidence
// files are then covered by the same backup as everything else on the box.
func FromEnv() Config {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_BACKEND")))
	if backend == "" {
		backend = "filesystem"
	}

	root := os.Getenv("STORAGE_PATH")
	if root == "" {
		root = "./data/evidence"
	}

	useSSL, _ := strconv.ParseBool(os.Getenv("MINIO_USE_SSL"))
	bucket := os.Getenv("MINIO_BUCKET")
	if bucket == "" {
		bucket = "npdms"
	}

	return Config{
		Backend:        backend,
		FilesystemRoot: root,
		MinioEndpoint:  os.Getenv("MINIO_ENDPOINT"),
		MinioAccessKey: os.Getenv("MINIO_ACCESS_KEY"),
		MinioSecretKey: os.Getenv("MINIO_SECRET_KEY"),
		MinioBucket:    bucket,
		MinioUseSSL:    useSSL,
	}
}

// Open builds the configured store.
func Open(cfg Config) (Store, error) {
	if cfg.Backend == "minio" {
		return NewMinioStore(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey,
			cfg.MinioBucket, cfg.MinioUseSSL)
	}
	return NewFilesystemStore(cfg.FilesystemRoot)
}
