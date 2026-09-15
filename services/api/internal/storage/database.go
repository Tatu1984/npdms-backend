package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultDatabaseMaxObjectBytes is the per-object cap of the database backend.
const DefaultDatabaseMaxObjectBytes int64 = 8 << 20

// ErrObjectTooLarge is returned by a backend that cannot hold an object of the
// size offered. Handlers answer it with 413 and its message.
var ErrObjectTooLarge = errors.New("object too large for the configured storage")

// DatabaseStore keeps objects in Postgres (table storage_objects, migration
// 000070), one row per key with the bytes in a bytea column.
//
// It exists for hosting where local disk does not persist and no object store
// is provisioned — the staging API on Vercel, whose filesystem is wiped between
// invocations. It is meant for small objects such as photographs and document
// scans: every object is held in memory while it is written and read, and each
// is capped (8 MB by default). Anything larger — body-worn camera recordings,
// most CCTV footage — is refused with a message saying object storage must be
// configured. It is not a substitute for MinIO/S3 on the edge server.
type DatabaseStore struct {
	db       *pgxpool.Pool
	maxBytes int64
}

// NewDatabaseStore checks that the objects table exists, so a missing
// migration is reported at start-up rather than on the first upload.
func NewDatabaseStore(db *pgxpool.Pool, maxBytes int64) (*DatabaseStore, error) {
	if db == nil {
		return nil, errors.New("database storage needs a database connection")
	}
	if maxBytes <= 0 {
		maxBytes = DefaultDatabaseMaxObjectBytes
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var exists bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('public.storage_objects') IS NOT NULL`).Scan(&exists); err != nil {
		return nil, fmt.Errorf("checking for the storage_objects table: %w", err)
	}
	if !exists {
		return nil, errors.New("table storage_objects does not exist; apply migration 000070 before selecting STORAGE_BACKEND=database")
	}
	return &DatabaseStore{db: db, maxBytes: maxBytes}, nil
}

func (s *DatabaseStore) Backend() string { return "database" }

// MaxObjectBytes reports the per-object cap.
func (s *DatabaseStore) MaxObjectBytes() int64 { return s.maxBytes }

func (s *DatabaseStore) tooLarge() error {
	return fmt.Errorf("%w: the file is larger than %d MB, the limit when files are kept in the database; object storage (MinIO or S3) must be configured to store larger files",
		ErrObjectTooLarge, s.maxBytes>>20)
}

func (s *DatabaseStore) Put(ctx context.Context, key string, r io.Reader, contentType string) (*Object, error) {
	if key == "" {
		return nil, errors.New("empty object key")
	}
	// Read at most one byte past the cap: enough to know the object is too
	// large without buffering an arbitrarily large upload.
	digest := sha256.New()
	var buf bytes.Buffer
	size, err := io.Copy(io.MultiWriter(&buf, digest), io.LimitReader(r, s.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if size > s.maxBytes {
		return nil, s.tooLarge()
	}
	sum := hex.EncodeToString(digest.Sum(nil))
	var stored time.Time
	err = s.db.QueryRow(ctx, `
		INSERT INTO storage_objects (key, content_type, size_bytes, sha256, data)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (key) DO UPDATE
		   SET content_type = EXCLUDED.content_type, size_bytes = EXCLUDED.size_bytes,
		       sha256 = EXCLUDED.sha256, data = EXCLUDED.data, stored_at = NOW()
		RETURNING stored_at
	`, key, contentType, size, sum, buf.Bytes()).Scan(&stored)
	if err != nil {
		return nil, err
	}
	return &Object{Key: key, Size: size, ContentType: contentType, SHA256: sum, StoredAt: stored}, nil
}

func (s *DatabaseStore) Get(ctx context.Context, key string) (io.ReadCloser, *Object, error) {
	var o Object
	var data []byte
	err := s.db.QueryRow(ctx, `
		SELECT key, content_type, size_bytes, sha256, stored_at, data FROM storage_objects WHERE key = $1
	`, key).Scan(&o.Key, &o.ContentType, &o.Size, &o.SHA256, &o.StoredAt, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), &o, nil
}

func (s *DatabaseStore) Stat(ctx context.Context, key string) (*Object, error) {
	var o Object
	err := s.db.QueryRow(ctx, `
		SELECT key, content_type, size_bytes, sha256, stored_at FROM storage_objects WHERE key = $1
	`, key).Scan(&o.Key, &o.ContentType, &o.Size, &o.SHA256, &o.StoredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *DatabaseStore) Delete(ctx context.Context, key string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM storage_objects WHERE key = $1`, key)
	return err
}

// Hash recomputes the digest from the bytes held now, not the stored value.
func (s *DatabaseStore) Hash(ctx context.Context, key string) (string, int64, error) {
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
