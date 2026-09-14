// Package storage holds evidence files.
//
// Two backends, chosen by configuration:
//
//   - filesystem: a directory on the edge server. The default, because a single
//     central server already has local disk, and one fewer service to run is one
//     fewer service to operate, secure and back up.
//   - minio: S3-compatible object storage, for deployments that want it.
//
// Both satisfy the same interface, so nothing above this package knows which is
// in use. Writes always return the SHA-256 of what was actually stored,
// computed while streaming rather than by reading the file back.
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrNotFound is returned when an object key does not exist.
var ErrNotFound = errors.New("object not found")

// Object describes a stored file.
type Object struct {
	Key         string
	Size        int64
	ContentType string
	// SHA256 of the bytes as stored, lowercase hex.
	SHA256   string
	StoredAt time.Time
}

// Store is the contract every backend satisfies.
type Store interface {
	// Put streams r into the store and returns what was written, including the
	// hash of the bytes as they passed through. The hash is never taken on
	// trust from the caller.
	Put(ctx context.Context, key string, r io.Reader, contentType string) (*Object, error)

	// Get opens an object for reading. The caller closes it.
	Get(ctx context.Context, key string) (io.ReadCloser, *Object, error)

	// Stat returns metadata without transferring the body.
	Stat(ctx context.Context, key string) (*Object, error)

	// Delete removes an object. Deleting something absent is not an error.
	Delete(ctx context.Context, key string) error

	// Hash re-reads an object and recomputes its digest. This is what makes an
	// integrity check meaningful: it measures the bytes now on disk, not a
	// value recorded when the file arrived.
	Hash(ctx context.Context, key string) (string, int64, error)

	// Backend names the implementation, for display and for provenance.
	Backend() string
}

/* ------------------------------- filesystem ------------------------------- */

// FilesystemStore keeps objects under a root directory, one file per key.
type FilesystemStore struct {
	root string
}

// NewFilesystemStore prepares a directory-backed store.
func NewFilesystemStore(root string) (*FilesystemStore, error) {
	if root == "" {
		return nil, errors.New("storage root must be set")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("creating storage root: %w", err)
	}
	return &FilesystemStore{root: root}, nil
}

func (s *FilesystemStore) Backend() string { return "filesystem" }

// path resolves a key inside the root and refuses to escape it. Keys come from
// request parameters, so traversal has to be impossible rather than unlikely.
func (s *FilesystemStore) path(key string) (string, error) {
	if key == "" {
		return "", errors.New("empty object key")
	}
	clean := filepath.Clean("/" + strings.ReplaceAll(key, "\\", "/"))
	full := filepath.Join(s.root, clean)

	rootAbs, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("object key escapes the storage root: %q", key)
	}
	return fullAbs, nil
}

func (s *FilesystemStore) Put(ctx context.Context, key string, r io.Reader, contentType string) (*Object, error) {
	full, err := s.path(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return nil, err
	}

	// Write to a temporary file first and rename on success, so a failed or
	// interrupted upload cannot leave a partial file where evidence should be.
	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	digest := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, digest), r)
	if err != nil {
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpName, full); err != nil {
		return nil, err
	}

	// Record the content type beside the object; the filesystem does not carry it.
	_ = os.WriteFile(full+".meta", []byte(contentType), 0o640)

	return &Object{
		Key:         key,
		Size:        size,
		ContentType: contentType,
		SHA256:      hex.EncodeToString(digest.Sum(nil)),
		StoredAt:    time.Now(),
	}, nil
}

func (s *FilesystemStore) Get(ctx context.Context, key string) (io.ReadCloser, *Object, error) {
	full, err := s.path(key)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	meta, err := s.Stat(ctx, key)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, meta, nil
}

func (s *FilesystemStore) Stat(ctx context.Context, key string) (*Object, error) {
	full, err := s.path(key)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	contentType := "application/octet-stream"
	if raw, err := os.ReadFile(full + ".meta"); err == nil && len(raw) > 0 {
		contentType = string(raw)
	}
	return &Object{
		Key:         key,
		Size:        info.Size(),
		ContentType: contentType,
		StoredAt:    info.ModTime(),
	}, nil
}

func (s *FilesystemStore) Delete(ctx context.Context, key string) error {
	full, err := s.path(key)
	if err != nil {
		return err
	}
	os.Remove(full + ".meta")
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *FilesystemStore) Hash(ctx context.Context, key string) (string, int64, error) {
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
