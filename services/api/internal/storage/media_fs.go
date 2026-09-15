package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FilesystemMediaStore keeps HLS files under root/<streamKey>/<file>. For
// development or a single disk-backed edge server — not Vercel, whose
// filesystem is ephemeral (OpenMedia refuses it there).
//
// Writes are atomic (temp file + rename) so a player never reads a
// half-written playlist or segment.
type FilesystemMediaStore struct {
	root string
}

func NewFilesystemMediaStore(root string) (*FilesystemMediaStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: MEDIA_FS_DIR is empty", ErrMediaNotConfigured)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("%w: cannot create %s: %v", ErrMediaNotConfigured, abs, err)
	}
	return &FilesystemMediaStore{root: abs}, nil
}

func (s *FilesystemMediaStore) Backend() string { return "fs" }

// dir and full re-validate key and name, so the store is safe even if a caller
// forgot to; nothing outside root is reachable.
func (s *FilesystemMediaStore) dir(streamKey string) (string, error) {
	if SafeMediaKey(streamKey) == "" {
		return "", errors.New("unsafe stream key")
	}
	return filepath.Join(s.root, streamKey), nil
}

func (s *FilesystemMediaStore) full(streamKey, file string) (string, error) {
	d, err := s.dir(streamKey)
	if err != nil {
		return "", err
	}
	if SafeMediaName(file) == "" {
		return "", errors.New("unsafe file name")
	}
	return filepath.Join(d, file), nil
}

func (s *FilesystemMediaStore) Put(ctx context.Context, streamKey, file string, body []byte) error {
	dest, err := s.full(streamKey, file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".put-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

func (s *FilesystemMediaStore) Get(ctx context.Context, streamKey, file string) (*MediaObject, error) {
	p, err := s.full(streamKey, file)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	body, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ct, _ := MediaContentType(file)
	return &MediaObject{Body: body, ContentType: ct, ModTime: info.ModTime()}, nil
}

func (s *FilesystemMediaStore) Delete(ctx context.Context, streamKey, file string) error {
	p, err := s.full(streamKey, file)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *FilesystemMediaStore) Purge(ctx context.Context, streamKey string) error {
	d, err := s.dir(streamKey)
	if err != nil {
		return err
	}
	// Playlist first, so a player stops asking for segments before they vanish.
	_ = os.Remove(filepath.Join(d, "index.m3u8"))
	return os.RemoveAll(d)
}
