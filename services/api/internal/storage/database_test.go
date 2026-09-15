package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs against a real database with migration 000070 applied:
// NPDMS_TEST_DATABASE_URL=postgres://localhost:5432/npdms_x go test ./internal/storage
func TestDatabaseStoreRoundTrip(t *testing.T) {
	url := os.Getenv("NPDMS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("NPDMS_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store, err := NewDatabaseStore(pool, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	body := make([]byte, 700_000)
	_, _ = rand.Read(body)
	want := sha256.Sum256(body)
	key := "test/database-store-roundtrip.bin"
	defer store.Delete(ctx, key)

	obj, err := store.Put(ctx, key, bytes.NewReader(body), "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	if obj.SHA256 != hex.EncodeToString(want[:]) || obj.Size != int64(len(body)) {
		t.Fatalf("put reported %s/%d", obj.SHA256, obj.Size)
	}
	r, meta, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if !bytes.Equal(got, body) || meta.SHA256 != obj.SHA256 || meta.ContentType != "application/octet-stream" {
		t.Fatal("bytes or metadata differ after the round trip")
	}
	if sum, size, err := store.Hash(ctx, key); err != nil || sum != obj.SHA256 || size != obj.Size {
		t.Fatalf("hash %s/%d/%v", sum, size, err)
	}

	// Over the cap: refused, with a message naming object storage, and nothing stored.
	big := make([]byte, (1<<20)+1)
	_, err = store.Put(ctx, "test/too-large.bin", bytes.NewReader(big), "video/mp4")
	if !errors.Is(err, ErrObjectTooLarge) || !bytes.Contains([]byte(err.Error()), []byte("object storage")) {
		t.Fatalf("expected ErrObjectTooLarge, got %v", err)
	}
	if _, err := store.Stat(ctx, "test/too-large.bin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oversized object was stored: %v", err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}
